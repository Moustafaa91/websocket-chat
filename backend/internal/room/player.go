package room

// Player slot names used across the hub and WebSocket protocol.
const (
	Player1 = "Player 1"
	Player2 = "Player 2"
)

// Slot returns the array index (0 or 1) for a player name.
func Slot(name string) int {
	if name == Player1 {
		return 0
	}
	return 1
}

// Partner returns the other player's name.
func Partner(name string) string {
	if name == Player1 {
		return Player2
	}
	return Player1
}
