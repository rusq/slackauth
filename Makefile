SHELL=/bin/sh
DEBUG=0

debug: DEBUG=1
debug: play
.PHONY: debug

play:
	DEBUG=$(DEBUG) go run ./cmd/playground
.PHONY: play
