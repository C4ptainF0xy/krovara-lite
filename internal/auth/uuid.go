package auth

import "github.com/google/uuid"

func uuidFromBytes(b [16]byte) uuid.UUID { return uuid.UUID(b) }
