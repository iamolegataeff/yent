/*
 * perception.h — SARTRE perception physics: utility events -> AML field pressure.
 *
 * A SARTRE utility (e.g. repo_monitor) emits change-events. This layer turns a scan's
 * worth of events into a small AML program — the "weather" the perception puts on the
 * field. Motion in the watched tree becomes VELOCITY; the scale of the change (and
 * whether Yent's own self-description, a README, moved) becomes PROPHECY horizon.
 *
 * The AML commands are am_exec-format strings (see yent/c/ariannamethod.h: VELOCITY
 * NOMOVE/WALK/RUN/BREATHE, PROPHECY 1..64). This layer EMITS the program; wiring it
 * onto the live field/limpha is the integration seam (Codex), not done here.
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
    int changed;         /* number of changed files (added+modified+removed) this scan */
    int readme_changed;  /* 1 if a README / self-description file moved */
} SartrePerception;

/* AM_State prophecy horizon is 1..64 (yent/c/ariannamethod.h). */
#define SARTRE_PROPHECY_MIN 1
#define SARTRE_PROPHECY_MAX 64

/* Summarise a repo_monitor JSON-lines event stream into a perception:
 * counts lines that carry a "kind" field, and flags a README path. Returns the
 * change count. (Minimal scan, not a full JSON parser — the event shape is fixed.) */
int sartre_perceive_from_events(const char *json_lines, SartrePerception *out);

/* Translate a perception into an AML program (newline-separated am_exec commands).
 * Quiet scan -> "VELOCITY NOMOVE\nPROPHECY 1"; motion -> "VELOCITY RUN\nPROPHECY N",
 * N = clamp(2 + changed + (readme?7:0), 1, 64). Writes into buf (NUL-terminated),
 * returns bytes written (excluding NUL), or -1 on bad args. */
int sartre_perceive_to_aml(const SartrePerception *p, char *buf, int max);

#ifdef __cplusplus
}
#endif

#endif /* SARTRE_PERCEPTION_H */
