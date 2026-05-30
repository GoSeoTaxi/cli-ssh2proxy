APP       := ssh2proxy
MAIN_PKG  := ./cmd/ssh2proxy
MODULE    := github.com/GoSeoTaxi/cli-ssh2proxy

TUN_PKG   := github.com/xjasonlyu/tun2socks/v2
TUN_VER   := v2.6.0

PLATFORMS := linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64

DIST      := bin
TUN_DIR   := internal/tun/bins
DIST_BINS := $(APP)-linux_amd64 $(APP)-linux_arm64 $(APP)-darwin_amd64 $(APP)-darwin_arm64 $(APP)-windows_amd64.exe
CHECKSUMS_FILE := $(DIST)/checksums.txt
VERSION   ?= dev
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.BuildDate=$(BUILD_DATE)
GOFLAGS   := -trimpath -ldflags='$(LDFLAGS)'

BINROOT   := $(shell go env GOBIN)
ifeq ($(strip $(BINROOT)),)
BINROOT   := $(shell go env GOPATH)/bin
endif

split = $(subst _, ,$1)
os    = $(word 1,$(call split,$1))
arch  = $(word 2,$(call split,$1))

.PHONY: all app tun update-readme checksums release-artifacts clean
all: tun app

app: tun update-readme

$(DIST)/$(APP)-%: | $(DIST)
	$(eval OS   := $(call os,$*))
	$(eval ARCH := $(call arch,$*))
	$(eval OUT  := $@$(if $(findstring windows,$(OS)),.exe,))

	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 \
	    go build $(GOFLAGS) -o $(OUT) $(MAIN_PKG)

	@echo "→ $(OUT)"

update-readme: $(addprefix $(DIST)/$(APP)-,$(PLATFORMS))
	@PLATFORMS="$(PLATFORMS)" APP="$(APP)" DIST="$(DIST)" \
	    python3 scripts/update_readme.py

checksums: $(addprefix $(DIST)/$(APP)-,$(PLATFORMS))
	@cd $(DIST) && shasum -a 256 $(DIST_BINS) > $(notdir $(CHECKSUMS_FILE))
	@echo "→ $(CHECKSUMS_FILE)"

release-artifacts: app checksums

tun: | $(TUN_DIR) $(addprefix $(TUN_DIR)/tun2socks-,$(PLATFORMS))

$(TUN_DIR)/tun2socks-%:
	GOOS=$(call os,$*) GOARCH=$(call arch,$*) \
	    go install $(TUN_PKG)@$(TUN_VER)

	$(eval EXT := $(if $(findstring windows,$*),.exe,))
	$(eval BIN1 := $(BINROOT)/$(call os,$*)_$(call arch,$*)/tun2socks$(EXT))
	$(eval BIN2 := $(BINROOT)/tun2socks$(EXT))

	@if [ -f "$(BIN1)" ]; then cp "$(BIN1)" $@$(EXT); \
	elif [ -f "$(BIN2)" ]; then cp "$(BIN2)" $@$(EXT); \
	else echo "tun2socks binary not found" >&2; exit 1; fi
	@echo "→ $@$(EXT)"

$(DIST) $(TUN_DIR):
	@mkdir -p $@

clean:
	rm -rf $(DIST) $(TUN_DIR)
	@echo "cleaned"
