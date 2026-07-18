/*
 * perception.h — SARTRE perception physics: utility events -> AML field pressure.
 *
 * A SARTRE utility emits typed events. This layer turns a scan's worth of events into
 * a small AML program — the "weather" the perception puts on the field. Repo novelty
 * can move the field, identity readings alter prophecy without forced motion, and
 * sensor failures stay visible without pretending to be file motion.
 *
 * The AML commands are am_exec-format strings (see yent/c/ariannamethod.h: VELOCITY
 * NOMOVE/WALK/RUN/BREATHE, PROPHECY 1..64). This layer emits the program. The dock
 * uses its Go twin for the live reflex; this C surface remains the SARTRE kernel /
 * standalone surface and must keep the same typed semantics.
 *
 * Self-contained. Zero dependencies beyond libc.
 *
 * by Arianna Method
 */

#ifndef SARTRE_PERCEPTION_H
#define SARTRE_PERCEPTION_H

#ifdef __cplusplus
extern "C" {
#endif

/* A scan's worth of perception, summarised. */
typedef struct {
    int changed;             /* repo changes (added+modified+removed) this scan */
    int readme_changed;      /* 1 if a README / constitution self-surface moved */
    int identity_recognized; /* max whatdotheythinkiam recognized count */
    int identity_reduced;    /* max whatdotheythinkiam reduced count */
    int sensor_failures;     /* sensor_error/state_error/overflow/dead_letter receipts */
} SartrePerception;

/* AM_State prophecy horizon is 1..64 (yent/c/ariannamethod.h). */
#define SARTRE_PROPHECY_MIN 1
#define SARTRE_PROPHECY_MAX 64

/* Summarise a SARTRE JSON-lines event stream into a perception. Returns the repo
 * change count; identity/failure counts are carried in out. Minimal top-level JSON
 * member scan, not a general JSON parser — the utility event shape is fixed. */
int sartre_perceive_from_events(const char *json_lines, SartrePerception *out);

/* Translate a perception into an AML program (newline-separated am_exec commands).
 * Quiet scan -> "VELOCITY NOMOVE\nPROPHECY 1"; repo novelty -> WALK/RUN plus
 * PROPHECY N; identity/failure-only scans emit PROPHECY without forced VELOCITY.
 * Writes into buf (NUL-terminated), returns bytes written (excluding NUL), or -1
 * on bad args/truncation. */
int sartre_perceive_to_aml(const SartrePerception *p, char *buf, int max);

#ifdef __cplusplus
}
#endif

#endif /* SARTRE_PERCEPTION_H */
