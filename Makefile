BIN := dim-weight-checker
VERSION ?= dev
DIST := dist

# GOOS/GOARCH pairs to cross-compile for a release. Windows gets a .exe
# suffix below; everything else is a bare binary tarred up with the
# platform name so it doesn't collide with the others in $(DIST).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build release clean

build:
	go build -o $(BIN) .

release: clean
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		out=$(BIN); \
		if [ "$$os" = "windows" ]; then out=$(BIN).exe; fi; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build -o $(DIST)/$$out . || exit 1; \
		tar -C $(DIST) -czf $(DIST)/$(BIN)-$(VERSION)-$$os-$$arch.tar.gz $$out; \
		rm $(DIST)/$$out; \
	done
	@cd $(DIST) && shasum -a 256 *.tar.gz > checksums.txt

clean:
	rm -rf $(DIST) $(BIN)
