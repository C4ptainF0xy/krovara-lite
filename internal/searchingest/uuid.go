package searchingest

import "github.com/google/uuid"

func parseUUID(s string) ([16]byte, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return [16]byte{}, false
	}
	return id, true
}

func uuidString(b [16]byte) string {
	return uuid.UUID(b).String()
}
