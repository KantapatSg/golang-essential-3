package identityv1

import (
	context "context"
	gen "github.com/KantapatSg/golang-essential-3/contracts/gen/go"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
}
type Empty struct{}

type IdentityServiceClient interface {
	Login(context.Context, *LoginRequest, ...grpc.CallOption) (*TokenResponse, error)
	Refresh(context.Context, *RefreshRequest, ...grpc.CallOption) (*TokenResponse, error)
	Logout(context.Context, *LogoutRequest, ...grpc.CallOption) (*Empty, error)
}
type identityServiceClient struct{ cc grpc.ClientConnInterface }

func NewIdentityServiceClient(cc grpc.ClientConnInterface) IdentityServiceClient {
	return &identityServiceClient{cc}
}
func (c *identityServiceClient) Login(ctx context.Context, in *LoginRequest, opts ...grpc.CallOption) (*TokenResponse, error) {
	out := new(TokenResponse)
	err := c.cc.Invoke(ctx, "/identity.v1.IdentityService/Login", in, out, opts...)
	return out, err
}
func (c *identityServiceClient) Refresh(ctx context.Context, in *RefreshRequest, opts ...grpc.CallOption) (*TokenResponse, error) {
	out := new(TokenResponse)
	err := c.cc.Invoke(ctx, "/identity.v1.IdentityService/Refresh", in, out, opts...)
	return out, err
}
func (c *identityServiceClient) Logout(ctx context.Context, in *LogoutRequest, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, "/identity.v1.IdentityService/Logout", in, out, opts...)
	return out, err
}

type IdentityServiceServer interface {
	Login(context.Context, *LoginRequest) (*TokenResponse, error)
	Refresh(context.Context, *RefreshRequest) (*TokenResponse, error)
	Logout(context.Context, *LogoutRequest) (*Empty, error)
	mustEmbedUnimplementedIdentityServiceServer()
}
type UnimplementedIdentityServiceServer struct{}

func (UnimplementedIdentityServiceServer) Login(context.Context, *LoginRequest) (*TokenResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method Login not implemented")
}
func (UnimplementedIdentityServiceServer) Refresh(context.Context, *RefreshRequest) (*TokenResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method Refresh not implemented")
}
func (UnimplementedIdentityServiceServer) Logout(context.Context, *LogoutRequest) (*Empty, error) {
	return nil, status.Error(codes.Unimplemented, "method Logout not implemented")
}
func (UnimplementedIdentityServiceServer) mustEmbedUnimplementedIdentityServiceServer() {}
func RegisterIdentityServiceServer(s grpc.ServiceRegistrar, srv IdentityServiceServer) {
	s.RegisterService(&IdentityService_ServiceDesc, srv)
}
func _Identity_Login_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LoginRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IdentityServiceServer).Login(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/identity.v1.IdentityService/Login"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IdentityServiceServer).Login(ctx, req.(*LoginRequest))
	}
	return interceptor(ctx, in, info, handler)
}
func _Identity_Refresh_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RefreshRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IdentityServiceServer).Refresh(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/identity.v1.IdentityService/Refresh"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IdentityServiceServer).Refresh(ctx, req.(*RefreshRequest))
	}
	return interceptor(ctx, in, info, handler)
}
func _Identity_Logout_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LogoutRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(IdentityServiceServer).Logout(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/identity.v1.IdentityService/Logout"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(IdentityServiceServer).Logout(ctx, req.(*LogoutRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var IdentityService_ServiceDesc = grpc.ServiceDesc{ServiceName: "identity.v1.IdentityService", HandlerType: (*IdentityServiceServer)(nil), Methods: []grpc.MethodDesc{{MethodName: "Login", Handler: _Identity_Login_Handler}, {MethodName: "Refresh", Handler: _Identity_Refresh_Handler}, {MethodName: "Logout", Handler: _Identity_Logout_Handler}}}

var _ = gen.JSONCodec{}
