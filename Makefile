APP       := ssh2proxy
MAIN_PKG  := ./cmd/ssh2proxy

TUN_PKG   := github.com/xjasonlyu/tun2socks/v2
TUN_VER   := v2.6.0

PLATFORMS := linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64

DIST      := bin
TUN_DIR   := internal/tun/bins
GOFLAGS   := -trimpath -ldflags='-s -w'

BINROOT   := $(shell go env GOBIN)
ifeq ($(strip $(BINROOT)),)
BINROOT   := $(shell go env GOPATH)/bin
endif

split = $(subst _, ,$1)
os    = $(word 1,$(call split,$1))
arch  = $(word 2,$(call split,$1))

.PHONY: all app tun clean
all: tun app

app: | $(DIST) $(addprefix $(DIST)/$(APP)-,$(PLATFORMS))

$(DIST)/$(APP)-%:
	$(eval OS   := $(call os,$*))
	$(eval ARCH := $(call arch,$*))
	$(eval OUT  := $@$(if $(findstring windows,$(OS)),.exe,))   # ← NEW

	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 \
	    go build $(GOFLAGS) -o $(OUT) $(MAIN_PKG)

	@echo "→ $(OUT)"

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