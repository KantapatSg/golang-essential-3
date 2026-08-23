package main

import (
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestPasswordHash(t *testing.T) {
	h, e := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if e != nil || bcrypt.CompareHashAndPassword(h, []byte("secret")) != nil {
		t.Fatal("bcrypt round trip failed")
	}
}
