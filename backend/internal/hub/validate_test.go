package hub

import "testing"

func TestValidateJoinRejectsUnknownCode(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	err := h.ValidateJoin("NOEXIST")
	if err == nil {
		t.Fatal("expected error for unknown room code")
	}
}

func TestValidateJoinAcceptsPendingRoom(t *testing.T) {
	h, cancel := newTestHub(t)
	defer cancel()

	code, err := h.ReserveCode()
	if err != nil {
		t.Fatalf("ReserveCode: %v", err)
	}

	if err := h.ValidateJoin(code); err != nil {
		t.Fatalf("ValidateJoin: %v", err)
	}
}
