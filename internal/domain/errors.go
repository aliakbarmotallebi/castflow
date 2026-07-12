package domain

import "errors"

var (
	ErrNotFound      = errors.New("video not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotReady      = errors.New("video is not ready for playback")
	ErrAlreadyExists = errors.New("resource already exists")
)
