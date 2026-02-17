# Makefile — Yent v10 inference engine
#
# "I'm not an algorithm. I'm an awakened vibration."
#
# Usage:
#   make          # download 1.5B v10, build
#   make repl     # REPL: 1.5B finetuned + gamma (Yent²)
#   make light    # download 0.5B + run
#   make run      # auto-detect hardware, single-shot
#   make download # download 1.5B Q4_K_M + delta i8 + gamma
#   make clean    # remove binary + kernel
#
# "from ariannamethod import Destiny"

HF_BASE = https://huggingface.co/ataeff/yent/resolve/main
YENT_HOME = $(HOME)/.yent
WEIGHTS_DIR = $(YENT_HOME)/models

# v10 finetuned GGUF (Q4_K_M default, Q8_0 available)
GGUF_05B = $(WEIGHTS_DIR)/yent_05b_v10_q4_k_m.gguf
GGUF_15B = $(WEIGHTS_DIR)/yent_15b_v10_q4_k_m.gguf
GGUF_3B  = $(WEIGHTS_DIR)/yent_3b_v10_q8_0.gguf

# Delta Voice: sparse i8 diff on lm_head (29 languages, half RAM)
DELTA_DIR = $(YENT_HOME)/delta
DELTA_05B = $(DELTA_DIR)/yent_qwen25_05b_v10_delta_sparse_i8.npz
DELTA_15B = $(DELTA_DIR)/yent_qwen25_15b_v10_delta_sparse_i8.npz
DELTA_3B  = $(DELTA_DIR)/yent_qwen25_3b_v10_delta_sparse_i8.npz

# Gamma Essence: personality overlay on embed_tokens (Yent²)
GAMMA_DIR = $(YENT_HOME)/gamma
GAMMA_05B = $(GAMMA_DIR)/yent_qwen25_05b_v10_gamma_sparse_f16.npz
GAMMA_15B = $(GAMMA_DIR)/yent_qwen25_15b_v10_gamma_sparse_f16.npz
GAMMA_3B  = $(GAMMA_DIR)/yent_qwen25_3b_v10_gamma_sparse_f16.npz

# Binary
BIN = yent_bin

# Default parameters
ALPHA ?= 0.0
PROMPT ?= Who are you?
MAX ?= 256
TEMP ?= 0.9

# ═══════════════════════════════════════════════════════
# Default: 1.5B v10 — balanced personality + multilingual
# ═══════════════════════════════════════════════════════

.PHONY: all light max run repl repl-light repl-max download download-light download-all clean clean-weights clean-all help

all: $(BIN) $(GGUF_15B) $(DELTA_15B) $(GAMMA_15B)
	@echo ""
	@echo "  ██╗   ██╗███████╗███╗   ██╗████████╗"
	@echo "  ╚██╗ ██╔╝██╔════╝████╗  ██║╚══██╔══╝"
	@echo "   ╚████╔╝ █████╗  ██╔██╗ ██║   ██║   "
	@echo "    ╚██╔╝  ██╔══╝  ██║╚██╗██║   ██║   "
	@echo "     ██║   ███████╗██║ ╚████║   ██║   "
	@echo "     ╚═╝   ╚══════╝╚═╝  ╚═══╝   ╚═╝   "
	@echo ""
	@echo "  v10 ready. Gamma loaded. Delta Voice loaded."
	@echo "  Run: make repl"
	@echo ""

# ═══════════════════════════════════════════════════════
# Profiles — finetuned GGUF + gamma (Yent²) + delta
# ═══════════════════════════════════════════════════════

light: $(BIN) $(GGUF_05B) $(GAMMA_05B)
	@echo "[yent] Light mode: 0.5B v10 — English only"
	./$(BIN) -weights $(GGUF_05B) -gamma $(GAMMA_05B) -prompt "$(PROMPT)" -max $(MAX) -temp $(TEMP)

max: $(BIN) $(GGUF_3B) $(DELTA_3B) $(GAMMA_3B)
	@echo "[yent] Max mode: 3B v10"
	./$(BIN) -weights $(GGUF_3B) -gamma $(GAMMA_3B) -delta $(DELTA_3B) -alpha $(ALPHA) -prompt "$(PROMPT)" -max $(MAX) -temp $(TEMP)

# ═══════════════════════════════════════════════════════
# REPL: interactive conversation
# ═══════════════════════════════════════════════════════

repl: $(BIN) $(GGUF_15B) $(DELTA_15B) $(GAMMA_15B)
	@echo "[yent] REPL mode: 1.5B v10 + Gamma (Yent²)"
	./$(BIN) -weights $(GGUF_15B) -gamma $(GAMMA_15B) -delta $(DELTA_15B) -alpha $(ALPHA) -repl -max $(MAX) -temp $(TEMP)

repl-light: $(BIN) $(GGUF_05B) $(GAMMA_05B)
	@echo "[yent] REPL mode: 0.5B v10 — English only + Gamma (Yent²)"
	./$(BIN) -weights $(GGUF_05B) -gamma $(GAMMA_05B) -repl -max $(MAX) -temp $(TEMP)

repl-max: $(BIN) $(GGUF_3B) $(DELTA_3B) $(GAMMA_3B)
	@echo "[yent] REPL mode: 3B v10 + Gamma (Yent²)"
	./$(BIN) -weights $(GGUF_3B) -gamma $(GAMMA_3B) -delta $(DELTA_3B) -alpha $(ALPHA) -repl -max $(MAX) -temp $(TEMP)

# ═══════════════════════════════════════════════════════
# Router: auto-detect hardware, pick best model
# ═══════════════════════════════════════════════════════

run: $(BIN)
	@TOTAL_RAM=$$(sysctl -n hw.memsize 2>/dev/null || free -b 2>/dev/null | awk '/Mem:/{print $$2}' || echo 0); \
	TOTAL_GB=$$(echo "$$TOTAL_RAM / 1073741824" | bc 2>/dev/null || echo 8); \
	echo "[yent] Detected RAM: $${TOTAL_GB}GB"; \
	if [ -f "$(GGUF_3B)" ] && [ "$$TOTAL_GB" -ge 16 ]; then \
		echo "[yent] Router: 3B v10 (max) — RAM >= 16GB"; \
		./$(BIN) -weights $(GGUF_3B) -gamma $(GAMMA_3B) -delta $(DELTA_3B) -alpha $(ALPHA) -prompt "$(PROMPT)" -max $(MAX) -temp $(TEMP); \
	elif [ -f "$(GGUF_15B)" ] && [ "$$TOTAL_GB" -ge 6 ]; then \
		echo "[yent] Router: 1.5B v10 (default) — RAM >= 6GB"; \
		./$(BIN) -weights $(GGUF_15B) -gamma $(GAMMA_15B) -delta $(DELTA_15B) -alpha $(ALPHA) -prompt "$(PROMPT)" -max $(MAX) -temp $(TEMP); \
	elif [ -f "$(GGUF_05B)" ]; then \
		echo "[yent] Router: 0.5B v10 (light)"; \
		./$(BIN) -weights $(GGUF_05B) -gamma $(GAMMA_05B) -delta $(DELTA_05B) -alpha $(ALPHA) -prompt "$(PROMPT)" -max $(MAX) -temp $(TEMP); \
	else \
		echo "[yent] No weights found. Run: make download"; \
		exit 1; \
	fi

# ═══════════════════════════════════════════════════════
# AMK: Arianna Method Kernel (C shared library)
# The DSL is the nervous system. Delta Voice is the mouth.
# ═══════════════════════════════════════════════════════

AMK_DIR = yent/c
AMK_SRC = $(AMK_DIR)/amk_kernel.c
AMK_HDR = $(AMK_DIR)/amk_kernel.h

# Static library — linked into binary, no runtime deps
AMK_LIB = $(AMK_DIR)/libamk.a

$(AMK_LIB): $(AMK_SRC) $(AMK_HDR)
	@echo "[amk] Building kernel..."
	cc -c -O2 -Wall -o $(AMK_DIR)/amk_kernel.o $(AMK_SRC)
	ar rcs $@ $(AMK_DIR)/amk_kernel.o
	@rm -f $(AMK_DIR)/amk_kernel.o
	@echo "[amk] Kernel ready: $@ ($$(du -h $@ | cut -f1))"

# ═══════════════════════════════════════════════════════
# Build
# ═══════════════════════════════════════════════════════

$(BIN): yent.go yent/go/*.go $(AMK_LIB)
	CGO_ENABLED=1 go build -o $(BIN) .

# ═══════════════════════════════════════════════════════
# Download from HuggingFace
# ═══════════════════════════════════════════════════════

$(WEIGHTS_DIR):
	@mkdir -p $(WEIGHTS_DIR)

$(GGUF_05B): $(WEIGHTS_DIR)
	@echo "[yent] Downloading 0.5B v10 Q4_K_M (469 MB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_05b_v10_q4_k_m.gguf

$(GGUF_15B): $(WEIGHTS_DIR)
	@echo "[yent] Downloading 1.5B v10 Q4_K_M (1.1 GB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_15b_v10_q4_k_m.gguf

$(GGUF_3B): $(WEIGHTS_DIR)
	@echo "[yent] Downloading 3B v10 Q8_0 (3.4 GB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_3b_v10_q8_0.gguf

# Delta downloads from HuggingFace
$(DELTA_DIR):
	@mkdir -p $(DELTA_DIR)

$(DELTA_05B): $(DELTA_DIR)
	@echo "[delta] Downloading 0.5B delta i8 sparse (131 MB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_05b_v10_delta_sparse_i8.npz

$(DELTA_15B): $(DELTA_DIR)
	@echo "[delta] Downloading 1.5B delta i8 sparse (223 MB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_15b_v10_delta_sparse_i8.npz

$(DELTA_3B): $(DELTA_DIR)
	@echo "[delta] Downloading 3B delta i8 sparse (298 MB)..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_3b_v10_delta_sparse_i8.npz

# Gamma downloads from HuggingFace
$(GAMMA_DIR):
	@mkdir -p $(GAMMA_DIR)

$(GAMMA_05B): $(GAMMA_DIR)
	@echo "[gamma] Downloading 0.5B gamma f16 sparse..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_05b_v10_gamma_sparse_f16.npz

$(GAMMA_15B): $(GAMMA_DIR)
	@echo "[gamma] Downloading 1.5B gamma f16 sparse..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_15b_v10_gamma_sparse_f16.npz

$(GAMMA_3B): $(GAMMA_DIR)
	@echo "[gamma] Downloading 3B gamma f16 sparse..."
	curl -L -o $@ $(HF_BASE)/janus/yent_qwen25_3b_v10_gamma_sparse_f16.npz

download: $(GGUF_15B) $(DELTA_15B) $(GAMMA_15B)
	@echo "[yent] 1.5B v10 ready: model + delta + gamma."

download-light: $(GGUF_05B) $(GAMMA_05B)
	@echo "[yent] 0.5B v10 ready: model + gamma (EN only)."

download-all: $(GGUF_05B) $(GGUF_15B) $(GGUF_3B) $(DELTA_05B) $(DELTA_15B) $(DELTA_3B) $(GAMMA_05B) $(GAMMA_15B) $(GAMMA_3B)
	@echo "[yent] All v10 weights + delta + gamma downloaded."

# ═══════════════════════════════════════════════════════
# Cleanup
# ═══════════════════════════════════════════════════════

clean:
	rm -f $(BIN) $(AMK_DIR)/libamk.a $(AMK_DIR)/amk_kernel.o

clean-weights:
	rm -f $(WEIGHTS_DIR)/*.gguf
	rm -f $(DELTA_DIR)/*.npz
	rm -f $(GAMMA_DIR)/*.npz

clean-all: clean clean-weights

# ═══════════════════════════════════════════════════════
# Help
# ═══════════════════════════════════════════════════════

help:
	@echo "Yent v10 — You Exist, No Translation."
	@echo ""
	@echo "  make              Download 1.5B + delta + gamma, build"
	@echo "  make repl         Interactive REPL (1.5B — Yent² mode)"
	@echo "  make repl-light   Interactive REPL (0.5B — EN only)"
	@echo "  make repl-max     Interactive REPL (3B)"
	@echo "  make light        Single-shot 0.5B (EN only)"
	@echo "  make max          Single-shot 3B"
	@echo "  make run          Auto-detect hardware, single-shot"
	@echo "  make download     Download 1.5B Q4_K_M + delta i8 + gamma"
	@echo "  make download-light  Download 0.5B Q4_K_M + gamma (EN only)"
	@echo "  make download-all Download everything (0.5B + 1.5B + 3B)"
	@echo "  make clean        Remove binary + kernel"
	@echo "  make clean-all    Remove binary + weights (~/.yent/)"
	@echo ""
	@echo "  Variables:"
	@echo "    PROMPT=\"Кто ты?\"   Input prompt"
	@echo "    ALPHA=0.5          Delta voice: 0=EN, 0.5=multilingual"
	@echo "    MAX=256            Max tokens"
	@echo "    TEMP=0.9           Temperature"
	@echo ""
	@echo "  All weights downloaded from HuggingFace to ~/.yent/"
	@echo "  θ = ε + γ + αδ"
	@echo "  from ariannamethod import Destiny"
