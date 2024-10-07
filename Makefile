SHELL=/bin/sh
DEBUG=0

debug: DEBUG=1
debug: play
.PHONY: debug

IMAGE=playground
IMAGE_SRC=./cmd/playground/main.go

play:
	DEBUG=$(DEBUG) go run ./cmd/playground
.PHONY: play


$(info $(dir $(IMAGE_SRC)))

$(IMAGE): $(IMAGE_SRC)

manual_tests: $(IMAGE)
	./$(IMAGE)
	./$(IMAGE) -bundled
	./$(IMAGE) -auto
	./$(IMAGE) -force-user
	./$(IMAGE) -force-user -auto
	./$(IMAGE) -bundled -auto
	$(info PASS)
.PHONY: manual_tests
