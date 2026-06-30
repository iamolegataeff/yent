# context_processor — SARTRE perception utility #2 (C + notorch).
# notorch is the install-path library (NOT a sibling checkout): /opt/homebrew.
# On macOS, libnotorch's BLAS resolves through Apple's Accelerate framework.

CC      ?= cc
CFLAGS  ?= -O2 -Wall -Wextra
NT_INC  ?= /opt/homebrew/include
NT_LIB  ?= /opt/homebrew/lib
LDLIBS  := -L$(NT_LIB) -lnotorch -lm
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
LDLIBS  += -framework Accelerate
endif

context_processor: context_processor.c
	$(CC) $(CFLAGS) $< -I$(NT_INC) $(LDLIBS) -o $@

test: context_processor.c
	$(CC) $(CFLAGS) -DSARTRE_CTX_TEST $< -I$(NT_INC) $(LDLIBS) -o context_processor_test
	./context_processor_test

clean:
	rm -f context_processor context_processor_test

.PHONY: test clean
