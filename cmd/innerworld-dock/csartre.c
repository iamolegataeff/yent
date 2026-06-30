/* Compile SARTRE perception (self-contained, libc-only) into the dock binary so the
 * live-field reflex can call sartre_perceive_* via cgo. Intra-repo include resolved
 * by the package's -I.../sartre CFLAGS. The standalone self-test main stays #ifdef'd
 * out (SARTRE_PERCEPTION_TEST undefined here). */
#include "perception.c"
