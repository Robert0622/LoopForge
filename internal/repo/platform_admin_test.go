package repo

import (
	"testing"

	"github.com/Tencent/LoopForge/internal/domain"
)

func TestNoBuiltinPlatformAdmins(t *testing.T) {
	for _, login := range []string{"admin", "alice", ""} {
		if IsBuiltinPlatformAdmin(login) {
			t.Fatalf("%q must not be a builtin platform admin", login)
		}
	}
}

func TestStoreIsPlatformAdminUsesRole(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	if s.IsPlatformAdmin("alice") {
		t.Fatal("unknown user must not be admin")
	}
	if err := s.PutUser(domain.User{LoginName: "alice", DisplayName: "Alice"}); err != nil {
		t.Fatal(err)
	}
	if s.IsPlatformAdmin("alice") {
		t.Fatal("regular user must not be admin")
	}
	alice, _ := s.UserByLoginName("alice")
	if err := s.SetUserPlatformRole(alice.ID, domain.PlatformRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if !s.IsPlatformAdmin("alice") {
		t.Fatal("explicit platform_role=admin must be honored")
	}
}

func TestEnvPlatformAdminsAreConfigurable(t *testing.T) {
	t.Setenv("LOOPFORGE_PLATFORM_ADMINS", "alicewang, boblee")
	if !IsAllowlistedPlatformAdmin("alicewang") || !IsAllowlistedPlatformAdmin("BobLee") {
		t.Fatal("env allowlist must authorize extra logins")
	}
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if !s.IsPlatformAdmin("alicewang") {
		t.Fatal("store must honor env platform admins")
	}
	granted, err := s.GrantPlatformAdmin("carol")
	if err != nil || granted.LoginName != "carol" || !granted.Revocable {
		t.Fatalf("grant = %+v err=%v", granted, err)
	}
	if !s.IsPlatformAdmin("carol") {
		t.Fatal("granted login must be admin")
	}
	if err := s.RevokePlatformAdmin("carol"); err != nil {
		t.Fatal(err)
	}
	if s.IsPlatformAdmin("carol") {
		t.Fatal("revoked login must lose admin")
	}
	if err := s.RevokePlatformAdmin("alicewang"); err == nil {
		t.Fatal("env-configured admin must not be revocable")
	}
}
