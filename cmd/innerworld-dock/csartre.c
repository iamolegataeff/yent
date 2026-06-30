/* Compile SARTRE perception (self-contained, libc-only) into the dock binary so the
 * live-field reflex can call sartre_perceive_* via cgo. Intra-repo include resolved
 * by the package's -I.../sartre CFLAGS. The standalone self-test main stays #ifdef'd
 * out (SARTRE_PERCEPTION_TEST undefined here). */
#include "perception.c"

/* Compile the SARTRE metrics hub into the dock too. HAS_DARIO suppresses the
 * standalone CLI main; the dock owns lifecycle through sartre_init/shutdown. */
#define HAS_DARIO 1
#include "sartre_kernel.c"
