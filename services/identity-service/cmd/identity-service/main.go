package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	gen "github.com/KantapatSg/golang-essential-3/contracts/gen/go"
	identityv1 "github.com/KantapatSg/golang-essential-3/contracts/gen/go/identity/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed migrations/001_init.sql
var migrationSQL string

type user struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	Role         string `gorm:"not null"`
	CreatedAt    time.Time
}
type identityServer struct {
	identityv1.UnimplementedIdentityServiceServer
	db                    *gorm.DB
	redis                 *redis.Client
	users                 map[string]user
	signer                *rsa.PrivateKey
	accessTTL, refreshTTL time.Duration
	devSessions           map[string]string
}

func (s *identityServer) Login(ctx context.Context, req *identityv1.LoginRequest) (*identityv1.TokenResponse, error) {
	// ตอบข้อความเดียวกันเมื่อ email หรือ password ผิด เพื่อลดการเปิดเผยว่ามีบัญชีใดอยู่ในระบบ
	var u user
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if s.db != nil {
		if err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error; err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
	} else {
		var ok bool
		u, ok = s.users[email]
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	return s.issue(ctx, u)
}
func (s *identityServer) issue(ctx context.Context, u user) (*identityv1.TokenResponse, error) {
	// Access token เป็น signed stateless credential อายุสั้น ส่วน refresh token เป็น session state
	// ที่เพิกถอนได้ใน Redis จึงรองรับ rotation และ logout ข้ามหลาย replica
	now := time.Now().UTC()
	access := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": u.ID, "role": u.Role, "type": "access", "iat": now.Unix(), "exp": now.Add(s.accessTTL).Unix()})
	at, e := access.SignedString(s.signer)
	if e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	buf := make([]byte, 32)
	if _, e = rand.Read(buf); e != nil {
		return nil, status.Error(codes.Internal, e.Error())
	}
	rt := base64.RawURLEncoding.EncodeToString(buf)
	if s.redis != nil {
		// Redis เก็บ refresh session พร้อม TTL และทำให้ rotation/logout ใช้ได้ข้าม replica
		if e = s.redis.Set(ctx, "session:"+rt, u.ID, s.refreshTTL).Err(); e != nil {
			return nil, status.Error(codes.Unavailable, "session store unavailable")
		}
	} else {
		// fallback นี้เปิดได้เฉพาะ DEV_MODE เพื่อไม่กลบ infra จริงโดยไม่ตั้งใจ
		s.devSessions[rt] = u.ID
	}
	return &identityv1.TokenResponse{AccessToken: at, RefreshToken: rt, TokenType: "Bearer", ExpiresIn: int64(s.accessTTL.Seconds()), UserID: u.ID, Role: u.Role}, nil
}
func (s *identityServer) sessionID(ctx context.Context, token string) (string, error) {
	if s.redis != nil {
		// GetDel ทำให้ refresh token ใช้ได้ครั้งเดียว การ refresh ซ้ำด้วย token เดิมจึงถูกปฏิเสธ
		id, e := s.redis.GetDel(ctx, "session:"+token).Result()
		if e == redis.Nil {
			return "", status.Error(codes.Unauthenticated, "invalid refresh token")
		}
		if e != nil {
			return "", status.Error(codes.Unavailable, "session store unavailable")
		}
		return id, nil
	}
	id, ok := s.devSessions[token]
	delete(s.devSessions, token)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "invalid refresh token")
	}
	return id, nil
}
func (s *identityServer) Refresh(ctx context.Context, req *identityv1.RefreshRequest) (*identityv1.TokenResponse, error) {
	id, e := s.sessionID(ctx, req.RefreshToken)
	if e != nil {
		return nil, e
	}
	var u user
	if s.db != nil {
		e = s.db.WithContext(ctx).First(&u, "id = ?", id).Error
	} else {
		for _, candidate := range s.users {
			if candidate.ID == id {
				u = candidate
				break
			}
		}
		if u.ID == "" {
			e = gorm.ErrRecordNotFound
		}
	}
	if e != nil {
		return nil, status.Error(codes.Unauthenticated, "unknown session")
	}
	return s.issue(ctx, u)
}
func (s *identityServer) Logout(ctx context.Context, req *identityv1.LogoutRequest) (*identityv1.Empty, error) {
	if s.redis != nil {
		if e := s.redis.Del(ctx, "session:"+req.RefreshToken).Err(); e != nil {
			return nil, status.Error(codes.Unavailable, "session store unavailable")
		}
	} else {
		delete(s.devSessions, req.RefreshToken)
	}
	return &identityv1.Empty{}, nil
}
func openDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	var last error
	for i := 0; i < 10; i++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			sqlDB, _ := db.DB()
			if err = sqlDB.PingContext(ctx); err == nil {
				return db, nil
			}
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, last
}
func loadKeys(privPath, pubPath string) (*rsa.PrivateKey, error) {
	// Local development สร้าง key pair ครั้งแรกได้ แต่ production ต้องรับ key จาก secret manager
	// และห้ามสร้าง key ใหม่ทุกครั้งที่ instance restart เพราะ token เดิมจะตรวจไม่ผ่าน
	if b, e := os.ReadFile(privPath); e == nil {
		if block, _ := pem.Decode(b); block != nil {
			if k, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
				return k, nil
			}
		}
	}
	k, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		return nil, e
	}
	_ = os.MkdirAll(filepath.Dir(privPath), 0700)
	_ = os.WriteFile(privPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}), 0600)
	pub, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	_ = os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}), 0644)
	return k, nil
}
func durationEnv(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if x, e := time.ParseDuration(v); e == nil {
			return x
		}
	}
	return d
}
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dsn := os.Getenv("IDENTITY_DB_DSN")
	if dsn == "" && !isDev() {
		log.Fatal("IDENTITY_DB_DSN is required")
	}
	var e error
	var db *gorm.DB
	devUsers := map[string]user{}
	if dsn != "" {
		db, e = openDB(ctx, dsn)
		if e != nil {
			log.Fatal(e)
		}
		// Baseline ใช้ embedded SQL เพื่อให้ตัวอย่างเริ่มง่าย; Phase 1 จะเพิ่ม versioned migration command
		// เพื่อให้ deploy และ rollback ตรวจสอบเวอร์ชัน schema ได้ชัดเจนขึ้น
		if e = db.Exec(migrationSQL).Error; e != nil {
			log.Fatal(e)
		}
		if e = seedUsers(db); e != nil {
			log.Fatal(e)
		}
	} else {
		devUsers = seedDevUsers()
	}
	raddr := os.Getenv("REDIS_ADDR")
	var rc *redis.Client
	if raddr != "" {
		rc = redis.NewClient(&redis.Options{Addr: raddr})
		if e = rc.Ping(ctx).Err(); e != nil {
			log.Fatal(e)
		}
	} else if !isDev() {
		log.Fatal("REDIS_ADDR is required")
	}
	priv := os.Getenv("JWT_PRIVATE_KEY_PATH")
	if priv == "" {
		priv = "deploy/keys/private.pem"
	}
	pub := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if pub == "" {
		pub = "deploy/keys/public.pem"
	}
	signer, e := loadKeys(priv, pub)
	if e != nil {
		log.Fatal(e)
	}
	srv := &identityServer{db: db, redis: rc, users: devUsers, signer: signer, accessTTL: durationEnv("ACCESS_TTL", 15*time.Minute), refreshTTL: durationEnv("REFRESH_TTL", 7*24*time.Hour), devSessions: map[string]string{}}
	addr := os.Getenv("IDENTITY_ADDR")
	if addr == "" {
		addr = ":50051"
	}
	lis, e := net.Listen("tcp", addr)
	if e != nil {
		log.Fatal(e)
	}
	g := grpc.NewServer(grpc.ForceServerCodec(gen.JSONCodec{}), grpc.ChainUnaryInterceptor(requestIDInterceptor, deadlineInterceptor))
	identityv1.RegisterIdentityServiceServer(g, srv)
	go func() { <-ctx.Done(); g.GracefulStop() }()
	log.Printf("identity-service listening on %s", addr)
	if e = g.Serve(lis); e != nil && !errors.Is(e, grpc.ErrServerStopped) {
		log.Fatal(e)
	}
}
func seedUsers(db *gorm.DB) error {
	for _, x := range []struct{ email, pw, role string }{{"admin@example.com", env("ADMIN_PASSWORD", "admin123"), "admin"}, {"member@example.com", env("MEMBER_PASSWORD", "member123"), "member"}} {
		var u user
		e := db.Where("email = ?", x.email).First(&u).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			h, e := bcrypt.GenerateFromPassword([]byte(x.pw), bcrypt.DefaultCost)
			if e != nil {
				return e
			}
			if e = db.Create(&user{ID: uuid.NewString(), Email: x.email, PasswordHash: string(h), Role: x.role}).Error; e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
	}
	return nil
}
func seedDevUsers() map[string]user {
	users := map[string]user{}
	for _, x := range []struct{ email, pw, role string }{{"admin@example.com", env("ADMIN_PASSWORD", "admin123"), "admin"}, {"member@example.com", env("MEMBER_PASSWORD", "member123"), "member"}} {
		h, _ := bcrypt.GenerateFromPassword([]byte(x.pw), bcrypt.MinCost)
		users[x.email] = user{ID: uuid.NewString(), Email: x.email, PasswordHash: string(h), Role: x.role}
	}
	return users
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func isDev() bool { return strings.EqualFold(os.Getenv("DEV_MODE"), "true") }
func requestIDInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Request ID เดียวกันต้องเดินทางจาก Browser -> Gateway -> gRPC เพื่อเชื่อม log ข้าม service
	if md, ok := metadata.FromIncomingContext(ctx); ok && len(md.Get("x-request-id")) > 0 {
		ctx = context.WithValue(ctx, "request_id", md.Get("x-request-id")[0])
	}
	return handler(ctx, req)
}
func deadlineInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// ปลายทางตั้ง default deadline เป็น safety net แต่ยังเคารพ deadline ที่ Gateway ส่งมา
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	return handler(ctx, req)
}
