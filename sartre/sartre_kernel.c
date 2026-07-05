/*
 * sartre_kernel.c — SARTRE: Meta-Linux Kernel for the Dario Equation
 *
 * "L'enfer, c'est les autres processus."
 *
 * Self-contained meta-Linux kernel. Compiles alone, compiles with dario.c,
 * compiles with or without the sartre/ extensions directory.
 *
 * Alone:      cc sartre_kernel.c -O2 -lm -o sartre_kernel
 * With dario: cc dario.c sartre_kernel.c -DHAS_SARTRE -DHAS_DARIO -O2 -lm -o dario
 *
 * Features:
 *   - SystemState aggregation (16 modules, inner world metrics)
 *   - Hardware detection + tongue tier routing (0.5B/1.5B/3B)
 *   - OverlayFS concept (immutable base + writable delta)
 *   - Namespace isolation (8 slots)
 *   - Package management (apk-inspired, 32 slots)
 *   - Event ringbuffer (8 events)
 *   - JSON state export (for web UI)
 *
 * Zero dependencies. Zero weights. Pure existence.
 *
 * by Arianna Method
 * הרזוננס לא נשבר
 */

#include "sartre_kernel.h"
#include <time.h>
#include <unistd.h>
#include <signal.h>
#include <errno.h>
#include <sys/wait.h>
#include <sys/resource.h>
#include <math.h>

#ifdef HAS_PERCEPTION
#include "perception.h"
#endif

#ifdef __APPLE__
#include <sys/sysctl.h>
#include <mach/mach.h>
#endif

/* ═══════════════════════════════════════════════════════════════════
 * GLOBAL STATE — l'être-pour-soi
 * ═══════════════════════════════════════════════════════════════════ */

static SartreSystemState sys = {0};
static int sartre_ready = 0;

/* ═══════════════════════════════════════════════════════════════════
 * LIFECYCLE — birth, existence, nothingness
 * ═══════════════════════════════════════════════════════════════════ */

static void sartre_detect_platform(void);
static int  sartre_detect_devices(void);

int sartre_init(const char *config_path) {
    (void)config_path;

    memset(&sys, 0, sizeof(SartreSystemState));
    sys.tongue_override = -1;
    sys.boot_time_ms = (int64_t)time(NULL) * 1000;

    sartre_detect_tongue_tier();
    sartre_detect_platform();
    sartre_detect_devices();
    sartre_ready = 1;

    /* register self as first module */
    sartre_update_module("sartre_kernel", SARTRE_MODULE_ACTIVE, 0.01f);

    fprintf(stderr, "[sartre] kernel initialized. host: %s/%s, %d CPU, RAM: %lld MB, tongue: %s, devices: %d\n",
            sys.os_name, sys.arch, sys.cpu_count,
            (long long)sys.total_ram_mb, sartre_tongue_tier_name(sys.tongue_tier), sys.device_count);
    return 0;
}

void sartre_shutdown(void) {
    if (!sartre_ready) return;
    for (int i = 0; i < sys.ns_count; i++)   /* don't leak live children on shutdown */
        if (sys.namespaces[i].spawned && sys.namespaces[i].active)
            sartre_ns_kill(i);
    sartre_notify_event("kernel_shutdown");
    sartre_ready = 0;
    fprintf(stderr, "[sartre] kernel shutdown\n");
}

int sartre_is_ready(void) {
    return sartre_ready;
}

/* ═══════════════════════════════════════════════════════════════════
 * EVENT RINGBUFFER — what happened
 * ═══════════════════════════════════════════════════════════════════ */

void sartre_notify_event(const char *event) {
    if (!sartre_ready || !event) return;

    if (sys.event_count < SARTRE_MAX_EVENTS) {
        strncpy(sys.last_events[sys.event_count], event, 255);
        sys.last_events[sys.event_count][255] = '\0';
        sys.event_count++;
    } else {
        for (int i = 0; i < SARTRE_MAX_EVENTS - 1; i++) {
            strncpy(sys.last_events[i], sys.last_events[i + 1], 255);
            sys.last_events[i][255] = '\0';
        }
        strncpy(sys.last_events[SARTRE_MAX_EVENTS - 1], event, 255);
        sys.last_events[SARTRE_MAX_EVENTS - 1][255] = '\0';
    }

    sys.step_count++;
}

/* ═══════════════════════════════════════════════════════════════════
 * METRIC UPDATES — the inner world
 * ═══════════════════════════════════════════════════════════════════ */

void sartre_update_inner_state(float trauma, float arousal, float valence,
                               float coherence, float prophecy_debt) {
    if (!sartre_ready) return;
    sys.trauma_level  = trauma;
    sys.arousal       = arousal;
    sys.valence       = valence;
    sys.coherence     = coherence;
    sys.prophecy_debt = prophecy_debt;
}

void sartre_update_schumann(float coherence, float phase) {
    if (!sartre_ready) return;
    sys.schumann_coherence = coherence;
    sys.schumann_phase     = phase;
}

void sartre_update_calendar(float tension, int is_shabbat) {
    if (!sartre_ready) return;
    sys.calendar_tension = tension;
    sys.is_shabbat       = is_shabbat;
}

void sartre_update_module(const char *name, SartreModuleStatus status, float load) {
    if (!sartre_ready || !name) return;

    int idx = -1;
    for (int i = 0; i < sys.module_count; i++) {
        if (strncmp(sys.modules[i].name, name, 63) == 0) {
            idx = i;
            break;
        }
    }

    if (idx == -1 && sys.module_count < SARTRE_MAX_MODULES) {
        idx = sys.module_count++;
        strncpy(sys.modules[idx].name, name, 63);
        sys.modules[idx].name[63] = '\0';
    }

    if (idx >= 0) {
        sys.modules[idx].status         = status;
        sys.modules[idx].load           = load;
        sys.modules[idx].last_active_ms = (int64_t)time(NULL) * 1000;
    }
}

SartreSystemState *sartre_get_state(void) {
    return &sys;
}

/* ═══════════════════════════════════════════════════════════════════
 * OVERLAY — R∪W filesystem
 *
 * base  = immutable: the formula, seed vocab, laws of nature
 * delta = writable:  learned co-occurrences, prophecies, bigrams
 *
 * overlay_ratio measures how far the organism has drifted from its
 * original state. 0.0 = pure origin, approaching 1.0 = mostly learned.
 * ═══════════════════════════════════════════════════════════════════ */

void sartre_overlay_init(int64_t base_size) {
    sys.overlay.base_size      = base_size;
    sys.overlay.delta_size     = 0;
    sys.overlay.overlay_writes = 0;
    sys.overlay.overlay_ratio  = 0.0f;
}

void sartre_overlay_write(int64_t bytes) {
    sys.overlay.delta_size += bytes;
    sys.overlay.overlay_writes++;

    int64_t total = sys.overlay.base_size + sys.overlay.delta_size;
    if (total > 0) {
        sys.overlay.overlay_ratio = (float)sys.overlay.delta_size / (float)total;
    }
}

float sartre_overlay_ratio(void) {
    return sys.overlay.overlay_ratio;
}

/* ═══════════════════════════════════════════════════════════════════
 * NAMESPACES — Leibniz monads, process isolation
 * ═══════════════════════════════════════════════════════════════════ */

int sartre_ns_create(const char *name, float cpu_share, float mem_limit_mb) {
    if (sys.ns_count >= SARTRE_MAX_NS) return -1;

    int id = sys.ns_count++;
    strncpy(sys.namespaces[id].name, name, 63);
    sys.namespaces[id].name[63] = '\0';
    sys.namespaces[id].pid          = id + 1;
    sys.namespaces[id].cpu_share    = cpu_share;
    sys.namespaces[id].mem_limit_mb = mem_limit_mb;
    sys.namespaces[id].active       = 1;
    sys.namespaces[id].spawned      = 0;   /* conceptual monad, not an OS process */

    char ev[128];
    snprintf(ev, sizeof(ev), "ns_create:%s", name);
    sartre_notify_event(ev);

    return id;
}

void sartre_ns_destroy(int ns_id) {
    if (ns_id < 0 || ns_id >= sys.ns_count) return;
    if (sys.namespaces[ns_id].spawned) {     /* real process: terminate + reap, don't leak */
        sartre_ns_kill(ns_id);
        return;
    }
    sys.namespaces[ns_id].active = 0;

    char ev[128];
    snprintf(ev, sizeof(ev), "ns_destroy:%s", sys.namespaces[ns_id].name);
    sartre_notify_event(ev);
}

SartreNamespace *sartre_ns_get(int ns_id) {
    if (ns_id < 0 || ns_id >= sys.ns_count) return NULL;
    return &sys.namespaces[ns_id];
}

extern char **environ;

/* EINTR-safe waitpid: a real reaper must not be fooled by an interrupting signal. */
static pid_t sartre_waitpid(pid_t pid, int *status, int flags) {
    pid_t r;
    do { r = waitpid(pid, status, flags); } while (r < 0 && errno == EINTR);
    return r;
}

int sartre_ns_alive(int ns_id);

/* Real process-slot. fork+setrlimit+execve — the limit must be set on the child
 * between fork and exec, so this is not posix_spawn. The child path uses only
 * execve (async-signal-safe) + a bare setrlimit syscall, so it is safe even when
 * the kernel is linked into a multithreaded host (the future Go/cgo runtime).
 * argv[0] must be a RESOLVED executable path — no PATH search. On Linux RLIMIT_AS
 * is a real address-space cap; on Darwin setrlimit(RLIMIT_AS) returns EINVAL
 * (unsupported — measured, see SARTRE_LOG), so mem_limit_mb does not bound memory
 * on macOS. A hard memory cap is the metalinux/Tier-V job. A utility must really
 * run before it can feed the field. */
int sartre_ns_spawn_piped(const char *name, char *const argv[], float mem_limit_mb, int *out_read_fd) {
    if (!name || !argv || !argv[0]) return -1;

    /* slot: refresh spawned slots before growing, so a long-lived supervisor
     * reclaims children that exited even if the host never called alive/kill. */
    int id = -1, grew = 0;
    for (int i = 0; i < sys.ns_count; i++) {
        if (sys.namespaces[i].spawned && sys.namespaces[i].active)
            (void)sartre_ns_alive(i);
        if (sys.namespaces[i].spawned && !sys.namespaces[i].active) { id = i; break; }
    }
    if (id < 0) {
        if (sys.ns_count >= SARTRE_MAX_NS) return -1;
        id = sys.ns_count++;
        grew = 1;
    }

    /* precompute the fd ceiling in the PARENT — sysconf is not async-signal-safe,
     * so the post-fork child must not call it; it only close()s (which is safe). */
    long maxfd = sysconf(_SC_OPEN_MAX);
    if (maxfd < 0 || maxfd > 4096) maxfd = 4096;

    int pipefd[2];
    if (out_read_fd) {
        if (pipe(pipefd) != 0) { if (grew) sys.ns_count--; return -1; }
    }

    pid_t pid = fork();
    if (pid < 0) {
        if (out_read_fd) { close(pipefd[0]); close(pipefd[1]); }
        if (grew) sys.ns_count--;           /* roll back the grown slot on failure */
        return -1;
    }

    if (pid == 0) {
        /* child: bound, then become the utility. Only async-signal-safe work here. */
        if (out_read_fd) {
            close(pipefd[0]);               /* child does not read its own pipe */
            if (dup2(pipefd[1], STDOUT_FILENO) < 0) _exit(126);  /* broken stdout: fail loud */
            if (pipefd[1] != STDOUT_FILENO) close(pipefd[1]);
        }
        /* close any other inherited fds so a utility never receives the host's
         * DB / model / socket descriptors (0/1/2 stay; the rest are not ours to keep).
         * maxfd was computed in the parent — only close() runs here (async-signal-safe). */
        for (int fd = 3; fd < (int)maxfd; fd++) close(fd);

        if (mem_limit_mb > 0.0f && mem_limit_mb <= 1048576.0f) {   /* guard float->rlim_t overflow */
            rlim_t bytes = (rlim_t)(mem_limit_mb * 1024.0f * 1024.0f);
            struct rlimit rl = { bytes, bytes };
            setrlimit(RLIMIT_AS, &rl);      /* real cap on Linux; EINVAL on Darwin (measured) */
        }
        execve(argv[0], argv, environ);     /* async-signal-safe; argv[0] = resolved path */
        _exit(127);                          /* exec failed */
    }

    /* parent: record the real child in the slot */
    if (out_read_fd) {
        close(pipefd[1]);                   /* parent only reads */
        *out_read_fd = pipefd[0];
    }

    memset(&sys.namespaces[id], 0, sizeof(SartreNamespace));  /* clear any reused stale data */
    strncpy(sys.namespaces[id].name, name, 63);
    sys.namespaces[id].name[63]     = '\0';
    sys.namespaces[id].pid          = (int)pid;
    sys.namespaces[id].cpu_share    = 0.0f;
    sys.namespaces[id].mem_limit_mb = mem_limit_mb;
    sys.namespaces[id].active       = 1;
    sys.namespaces[id].spawned      = 1;

    char ev[128];
    snprintf(ev, sizeof(ev), "ns_spawn:%s pid=%d", name, (int)pid);
    sartre_notify_event(ev);

    return id;
}

/* Spawn with the utility's stdout inherited (no pipe back). Thin wrapper. */
int sartre_ns_spawn(const char *name, char *const argv[], float mem_limit_mb) {
    return sartre_ns_spawn_piped(name, argv, mem_limit_mb, NULL);
}

int sartre_ns_alive(int ns_id) {
    if (ns_id < 0 || ns_id >= sys.ns_count) return 0;
    SartreNamespace *ns = &sys.namespaces[ns_id];
    if (!ns->spawned) return ns->active;     /* monad: stored flag is its truth */
    if (!ns->active)  return 0;

    int status;
    pid_t r = sartre_waitpid((pid_t)ns->pid, &status, WNOHANG);
    if (r == 0) return 1;                     /* still running, not reaped */
    if (r == (pid_t)ns->pid || (r < 0 && errno == ECHILD)) {
        ns->active = 0;                       /* exited (reaped now or already gone) */
        char ev[128];
        snprintf(ev, sizeof(ev), "ns_exit:%s pid=%d", ns->name, ns->pid);
        sartre_notify_event(ev);
        return 0;
    }
    /* any other waitpid error on our own child: treat as dead — do NOT probe a
     * possibly-reused pid with kill(pid,0) (would touch an unrelated process). */
    ns->active = 0;
    return 0;
}

void sartre_ns_kill(int ns_id) {
    if (ns_id < 0 || ns_id >= sys.ns_count) return;
    SartreNamespace *ns = &sys.namespaces[ns_id];

    if (!ns->spawned) {                       /* conceptual monad: same as destroy */
        ns->active = 0;
        char ev[128];
        snprintf(ev, sizeof(ev), "ns_destroy:%s", ns->name);
        sartre_notify_event(ev);
        return;
    }

    if (ns->active && ns->pid > 0) {
        int status, reaped = 0;
        pid_t r;
        /* preflight: it may have already exited — reap without signalling */
        r = sartre_waitpid((pid_t)ns->pid, &status, WNOHANG);
        if (r == (pid_t)ns->pid || (r < 0 && errno == ECHILD)) reaped = 1;

        if (!reaped) {
            kill((pid_t)ns->pid, SIGTERM);
            for (int i = 0; i < 50; i++) {    /* up to ~500ms grace, reaping as we wait */
                r = sartre_waitpid((pid_t)ns->pid, &status, WNOHANG);
                if (r == (pid_t)ns->pid || (r < 0 && errno == ECHILD)) { reaped = 1; break; }
                struct timespec ts = { 0, 10 * 1000 * 1000 };  /* 10ms */
                nanosleep(&ts, NULL);
            }
        }
        /* only escalate if NOT yet reaped — never signal a pid we already reaped
         * (it could have been reused by an unrelated process). */
        if (!reaped) {
            kill((pid_t)ns->pid, SIGKILL);
            sartre_waitpid((pid_t)ns->pid, &status, 0);
        }
    }

    ns->active = 0;
    char ev[128];
    snprintf(ev, sizeof(ev), "ns_kill:%s pid=%d", ns->name, ns->pid);
    sartre_notify_event(ev);
}

/* ═══════════════════════════════════════════════════════════════════
 * PACKAGES — apk-inspired package management
 *
 * Not a full package manager. A registry of what's available and
 * what's installed. The kernel knows its own composition.
 * ═══════════════════════════════════════════════════════════════════ */

int sartre_pkg_register(const char *name, const char *version, int size_kb) {
    if (sys.pkg_count >= SARTRE_MAX_PACKAGES) return -1;

    int id = sys.pkg_count++;
    strncpy(sys.packages[id].name, name, 63);
    sys.packages[id].name[63] = '\0';
    strncpy(sys.packages[id].version, version, 31);
    sys.packages[id].version[31] = '\0';
    sys.packages[id].installed = 0;
    sys.packages[id].size_kb   = size_kb;
    return id;
}

int sartre_pkg_find(const char *name) {
    for (int i = 0; i < sys.pkg_count; i++) {
        if (strncmp(sys.packages[i].name, name, 63) == 0) return i;
    }
    return -1;
}

int sartre_pkg_install(const char *name) {
    int id = sartre_pkg_find(name);
    if (id < 0) return -1;
    if (sys.packages[id].installed) return 0; /* already installed */

    sys.packages[id].installed = 1;

    char ev[128];
    snprintf(ev, sizeof(ev), "pkg_install:%s@%s", name, sys.packages[id].version);
    sartre_notify_event(ev);

    /* track in overlay */
    sartre_overlay_write((int64_t)sys.packages[id].size_kb * 1024);

    return 0;
}

int sartre_pkg_remove(const char *name) {
    int id = sartre_pkg_find(name);
    if (id < 0) return -1;
    if (!sys.packages[id].installed) return 0;

    sys.packages[id].installed = 0;

    char ev[128];
    snprintf(ev, sizeof(ev), "pkg_remove:%s", name);
    sartre_notify_event(ev);

    return 0;
}

void sartre_pkg_list(void) {
    printf("\n=== SARTRE PACKAGES ===\n");
    for (int i = 0; i < sys.pkg_count; i++) {
        printf("  %s %s [%s] %dKB\n",
               sys.packages[i].installed ? "*" : " ",
               sys.packages[i].name,
               sys.packages[i].version,
               sys.packages[i].size_kb);
    }
    int installed_count = 0;
    for (int i = 0; i < sys.pkg_count; i++)
        if (sys.packages[i].installed) installed_count++;
    printf("  %d packages, %d installed\n\n", sys.pkg_count, installed_count);
}

/* ═══════════════════════════════════════════════════════════════════
 * TONGUE ROUTING — hardware-based model selection
 * ═══════════════════════════════════════════════════════════════════ */

static int64_t detect_total_ram_mb(void) {
#ifdef __APPLE__
    int64_t memsize;
    size_t len = sizeof(memsize);
    if (sysctlbyname("hw.memsize", &memsize, &len, NULL, 0) == 0)
        return memsize / (1024 * 1024);
    return 4096;
#else
    FILE *f = fopen("/proc/meminfo", "r");
    if (f) {
        int64_t total_kb = 0;
        char line[256];
        while (fgets(line, sizeof(line), f)) {
            if (strncmp(line, "MemTotal:", 9) == 0) {
                sscanf(line + 9, " %lld", &total_kb);
                break;
            }
        }
        fclose(f);
        if (total_kb > 0) return total_kb / 1024;
    }
    return 4096;
#endif
}

/* ── Portable platform auto-detection (Darwin / Linux / robot SBC) ──
 * The hardware describes itself; SARTRE adapts. uname gives arch+OS,
 * a portable branch counts CPUs. Same code on a Mac, a Linux server,
 * or a moving robot. */
#include <sys/utsname.h>
static void sartre_detect_platform(void) {
    struct utsname u;
    if (uname(&u) == 0) {
        snprintf(sys.arch,    sizeof sys.arch,    "%s", u.machine);
        snprintf(sys.os_name, sizeof sys.os_name, "%s", u.sysname);
    } else {
        snprintf(sys.arch,    sizeof sys.arch,    "%s", "unknown");
        snprintf(sys.os_name, sizeof sys.os_name, "%s", "unknown");
    }
#ifdef __APPLE__
    int ncpu = 0; size_t len = sizeof(ncpu);
    if (sysctlbyname("hw.ncpu", &ncpu, &len, NULL, 0) == 0) sys.cpu_count = ncpu;
#else
    long n = sysconf(_SC_NPROCESSORS_ONLN);
    if (n > 0) sys.cpu_count = (int)n;
#endif
}

/* ═══════════════════════════════════════════════════════════════════
 * LIVE METRICS HUB — SARTRE concentrates the body's telemetry
 * ═══════════════════════════════════════════════════════════════════ */

static float clamp01(float v) { return v < 0.0f ? 0.0f : (v > 1.0f ? 1.0f : v); }

/* Sample real cpu_load (load average / cpu count) + memory_pressure (used/total).
 * On failure a field keeps its previous value — never crashes, never fabricates. */
void sartre_sample_load(void) {
    double la[1];
    if (getloadavg(la, 1) == 1 && sys.cpu_count > 0) {
        sys.cpu_load = clamp01((float)(la[0] / (double)sys.cpu_count));
    }

#ifdef __APPLE__
    vm_statistics64_data_t vm;
    mach_msg_type_number_t cnt = HOST_VM_INFO64_COUNT;
    long page = sysconf(_SC_PAGESIZE);
    mach_port_t host = mach_host_self();
    if (page > 0 && sys.total_ram_mb > 0 &&
        host_statistics64(host, HOST_VM_INFO64, (host_info64_t)&vm, &cnt) == KERN_SUCCESS) {
        /* free + inactive (reclaimable) ≈ available, as `vm_stat` reports */
        uint64_t avail = ((uint64_t)vm.free_count + (uint64_t)vm.inactive_count) * (uint64_t)page;
        double total = (double)sys.total_ram_mb * 1024.0 * 1024.0;
        if (total > 0.0) sys.memory_pressure = clamp01((float)(1.0 - (double)avail / total));
    }
    mach_port_deallocate(mach_task_self(), host);  /* release mach_host_self() send right */
#else
    FILE *f = fopen("/proc/meminfo", "r");
    if (f) {
        long total_kb = 0, avail_kb = 0;
        char line[256];
        while (fgets(line, sizeof line, f)) {
            if (total_kb == 0) sscanf(line, "MemTotal: %ld kB", &total_kb);
            if (avail_kb == 0) sscanf(line, "MemAvailable: %ld kB", &avail_kb);
        }
        fclose(f);
        if (total_kb > 0 && avail_kb > 0)  /* keep prior value if either is unparsed */
            sys.memory_pressure = clamp01(1.0f - (float)avail_kb / (float)total_kb);
    }
#endif
}

/* Find "key": <number> in a flat JSON object; returns 1 and writes *out if found
 * and finite. Minimal scan — the event shape is fixed, not arbitrary JSON. */
static int json_get_float(const char *json, const char *key, float *out) {
    char pat[64];
    int pn = snprintf(pat, sizeof pat, "\"%s\"", key);
    if (pn <= 0 || pn >= (int)sizeof pat) return 0;
    const char *p = json;
    while ((p = strstr(p, pat)) != NULL) {
        /* the key must START a top-level member: the char before "key" is { , or
         * whitespace. This rejects the key appearing inside a quoted value string
         * (e.g. {"note":"\"debt\":9"} must not set debt). */
        char prev = (p == json) ? '{' : p[-1];
        const char *q = p + pn;               /* then optional whitespace, then ':' */
        if (prev == '{' || prev == ',' || prev == ' ' || prev == '\t' ||
            prev == '\n' || prev == '\r') {
            while (*q == ' ' || *q == '\t' || *q == '\n' || *q == '\r') q++;
            if (*q == ':') {
                char *end = NULL;
                double v = strtod(q + 1, &end);
                if (end != q + 1 && isfinite(v)) { *out = (float)v; return 1; }
            }
        }
        p += pn;                              /* keep scanning for a boundary-correct match */
    }
    return 0;
}

/* Reciprocal seam: the field/innerworld pushes its inner weather into the hub.
 * Receiver only — the sender lives on the field side (same shape as utilities
 * emitting JSON to the field, now reversed). */
void sartre_ingest_metrics_json(const char *json) {
    if (!json) return;
    float v;
    int touched = 0;
    if (json_get_float(json, "debt", &v))               { sys.prophecy_debt = v; touched++; }
    if (json_get_float(json, "coherence", &v))          { sys.coherence = v; touched++; }
    if (json_get_float(json, "entropy", &v))            { sys.entropy = v; touched++; }
    if (json_get_float(json, "valence", &v))            { sys.valence = v; touched++; }
    if (json_get_float(json, "arousal", &v))            { sys.arousal = v; touched++; }
    if (json_get_float(json, "trauma", &v))             { sys.trauma_level = v; touched++; }
    if (json_get_float(json, "warmth", &v))             { sys.warmth = v; touched++; }
    if (json_get_float(json, "flow", &v))               { sys.flow = v; touched++; }
    if (json_get_float(json, "memory_field_score", &v))    { sys.memory_field_score = v; touched++; }
    if (json_get_float(json, "memory_field_prophecy", &v)) { sys.memory_field_prophecy = v; touched++; }
    if (json_get_float(json, "memory_field_step", &v))     { sys.memory_field_step = v; touched++; }
    if (json_get_float(json, "schumann_coherence", &v)) { sys.schumann_coherence = v; touched++; }
    if (json_get_float(json, "schumann_phase", &v))     { sys.schumann_phase = v; touched++; }
    if (touched) sartre_notify_event("metrics_ingest");
}

/* ── Device auto-detection — forward-looking (robot camera/motors/sensors).
 * Empty on a Mac host (no peripherals we own). On a Linux robot/SBC this is
 * where /dev video / motor-serial / i2c-sensor nodes get registered. The
 * structure and the call site are live now; the Linux probe fills in when
 * the robot host exists. Returns the number of devices found. */
static int sartre_detect_devices(void) {
    sys.device_count = 0;
#ifndef __APPLE__
    /* Linux/robot host: scan /dev for camera (video*), motor (tty*/serial),
     * sensor (i2c-*) nodes and register each as a SartreDevice. To be filled
     * when the robot host lands; the slot array is ready (SARTRE_MAX_DEVICES). */
#endif
    return sys.device_count;
}

SartreTongueTier sartre_detect_tongue_tier(void) {
    int64_t ram_mb = detect_total_ram_mb();
    sys.total_ram_mb = ram_mb;

    if (sys.tongue_override >= 0) {
        sys.tongue_tier = (SartreTongueTier)sys.tongue_override;
        return sys.tongue_tier;
    }

    if (ram_mb >= 8000)      sys.tongue_tier = SARTRE_TONGUE_3B;
    else if (ram_mb >= 4000) sys.tongue_tier = SARTRE_TONGUE_15B;
    else                     sys.tongue_tier = SARTRE_TONGUE_05B;

    return sys.tongue_tier;
}

void sartre_set_tongue_override(SartreTongueTier tier) {
    sys.tongue_override = (int)tier;
    sys.tongue_tier     = tier;
}

void sartre_clear_tongue_override(void) {
    sys.tongue_override = -1;
    sartre_detect_tongue_tier();
}

SartreTongueTier sartre_get_tongue_tier(void) {
    return sys.tongue_tier;
}

const char *sartre_tongue_tier_name(SartreTongueTier tier) {
    switch (tier) {
        case SARTRE_TONGUE_05B: return "0.5B";
        case SARTRE_TONGUE_15B: return "1.5B";
        case SARTRE_TONGUE_3B:  return "3B";
        default: return "unknown";
    }
}

int64_t sartre_get_total_ram_mb(void) {
    return sys.total_ram_mb;
}

/* ═══════════════════════════════════════════════════════════════════
 * MODEL ROUTING — agnostic, DoE-style auto-detection
 *
 * Any model file. Any size. Any architecture.
 * Sartre profiles it from the file itself:
 *   .bin  → param_count = file_size / 4 (float32)
 *   .gguf → read GGUF metadata header for actual param count
 *   .pt   → param_count = file_size / 4 (approximation)
 *
 * Runtime memory estimated. Fit-in-RAM checked against 80% threshold.
 * Best model = largest that fits.
 * ═══════════════════════════════════════════════════════════════════ */

static int64_t file_size_bytes(const char *path) {
    FILE *f = fopen(path, "rb");
    if (!f) return -1;
    fseek(f, 0, SEEK_END);
    int64_t size = ftell(f);
    fclose(f);
    return size;
}

static int detect_gguf_params(const char *path, SartreModelProfile *out) {
    /* GGUF magic: 0x46554747 ("GGUF") at offset 0 */
    FILE *f = fopen(path, "rb");
    if (!f) return 0;

    uint32_t magic = 0;
    if (fread(&magic, 4, 1, f) != 1) { fclose(f); return 0; }

    if (magic != 0x46554747) { fclose(f); return 0; } /* not GGUF */

    /* GGUF v3: version(4) + tensor_count(8) + metadata_kv_count(8) */
    uint32_t version = 0;
    fread(&version, 4, 1, f);

    uint64_t tensor_count = 0, kv_count = 0;
    if (version >= 3) {
        fread(&tensor_count, 8, 1, f);
        fread(&kv_count, 8, 1, f);
    } else {
        uint32_t tc32 = 0, kv32 = 0;
        fread(&tc32, 4, 1, f);
        fread(&kv32, 4, 1, f);
        tensor_count = tc32;
        kv_count = kv32;
    }

    (void)kv_count; /* we'd need to parse KV pairs for exact dims */

    fclose(f);

    /* Estimate: typical GGUF packs ~2 bytes per param (Q4/Q5 average) */
    if (out->file_size_bytes > 0 && tensor_count > 0) {
        out->param_count = out->file_size_bytes / 2; /* rough for quantized */
        out->runtime_mb = (float)(out->file_size_bytes + 512 * 1024 * 1024) / (1024.0f * 1024.0f);
    }
    return 1;
}

static void profile_model_file(SartreModelProfile *p) {
    p->file_size_bytes = file_size_bytes(p->path);
    if (p->file_size_bytes <= 0) return;

    const char *ext = strrchr(p->path, '.');
    if (!ext) ext = "";

    if (strcmp(ext, ".gguf") == 0 || strcmp(ext, ".GGUF") == 0) {
        detect_gguf_params(p->path, p);
    } else if (strcmp(ext, ".bin") == 0) {
        /* Raw float32 weights: params = file_size / 4 */
        p->param_count = p->file_size_bytes / 4;
        p->runtime_mb = (float)p->file_size_bytes / (1024.0f * 1024.0f) * 1.2f;
    } else if (strcmp(ext, ".pt") == 0 || strcmp(ext, ".pth") == 0) {
        /* PyTorch checkpoint: rough estimate, mixed types */
        p->param_count = p->file_size_bytes / 3; /* avg ~3 bytes per param */
        p->runtime_mb = (float)p->file_size_bytes / (1024.0f * 1024.0f) * 1.5f;
    } else {
        /* Unknown format: assume float32 */
        p->param_count = p->file_size_bytes / 4;
        p->runtime_mb = (float)p->file_size_bytes / (1024.0f * 1024.0f) * 1.2f;
    }

    /* Check if it fits */
    p->fits_in_ram = (p->runtime_mb < sys.total_ram_mb * 0.8f) ? 1 : 0;
}

int sartre_model_register(const char *name, const char *path) {
    if (!sartre_ready || !name || !path) return -1;
    if (sys.model_count >= SARTRE_MAX_MODELS) return -1;

    /* Check if already registered */
    for (int i = 0; i < sys.model_count; i++) {
        if (strncmp(sys.models[i].name, name, 63) == 0) {
            /* Update path, re-profile */
            strncpy(sys.models[i].path, path, 255);
            sys.models[i].path[255] = '\0';
            profile_model_file(&sys.models[i]);

            char ev[128];
            snprintf(ev, sizeof(ev), "model_update:%s %lldM params",
                     name, (long long)(sys.models[i].param_count / 1000000));
            sartre_notify_event(ev);
            return i;
        }
    }

    int idx = sys.model_count++;
    memset(&sys.models[idx], 0, sizeof(SartreModelProfile));
    strncpy(sys.models[idx].name, name, 63);
    sys.models[idx].name[63] = '\0';
    strncpy(sys.models[idx].path, path, 255);
    sys.models[idx].path[255] = '\0';

    profile_model_file(&sys.models[idx]);

    char ev[128];
    snprintf(ev, sizeof(ev), "model_register:%s %lldM params %.0fMB fits=%d",
             name, (long long)(sys.models[idx].param_count / 1000000),
             sys.models[idx].runtime_mb, sys.models[idx].fits_in_ram);
    sartre_notify_event(ev);

    /* Also update legacy tongue tier based on best model */
    const SartreModelProfile *best = sartre_model_best();
    if (best && sys.tongue_override < 0) {
        if (best->param_count >= 2000000000LL)      sys.tongue_tier = SARTRE_TONGUE_3B;
        else if (best->param_count >= 1000000000LL)  sys.tongue_tier = SARTRE_TONGUE_15B;
        else                                          sys.tongue_tier = SARTRE_TONGUE_05B;
    }

    return idx;
}

const SartreModelProfile *sartre_model_get(const char *name) {
    if (!name) return NULL;
    for (int i = 0; i < sys.model_count; i++) {
        if (strncmp(sys.models[i].name, name, 63) == 0)
            return &sys.models[i];
    }
    return NULL;
}

const SartreModelProfile *sartre_model_best(void) {
    const SartreModelProfile *best = NULL;
    int64_t best_params = -1;

    for (int i = 0; i < sys.model_count; i++) {
        if (sys.models[i].fits_in_ram && sys.models[i].param_count > best_params) {
            best = &sys.models[i];
            best_params = sys.models[i].param_count;
        }
    }

    /* Fallback: if nothing fits, pick smallest */
    if (!best && sys.model_count > 0) {
        int64_t smallest = INT64_MAX;
        for (int i = 0; i < sys.model_count; i++) {
            if (sys.models[i].param_count < smallest) {
                smallest = sys.models[i].param_count;
                best = &sys.models[i];
            }
        }
    }

    return best;
}

void sartre_model_set_loaded(const char *name, int loaded) {
    for (int i = 0; i < sys.model_count; i++) {
        if (strncmp(sys.models[i].name, name, 63) == 0) {
            sys.models[i].loaded = loaded;
            sartre_update_module(name, loaded ? SARTRE_MODULE_ACTIVE : SARTRE_MODULE_IDLE,
                                 loaded ? sys.models[i].runtime_mb / (float)sys.total_ram_mb : 0.0f);
            char ev[128];
            snprintf(ev, sizeof(ev), "model_%s:%s", loaded ? "load" : "unload", name);
            sartre_notify_event(ev);
            return;
        }
    }
}

void sartre_model_list(void) {
    printf("\n=== SARTRE MODELS ===\n");
    for (int i = 0; i < sys.model_count; i++) {
        SartreModelProfile *m = &sys.models[i];
        printf("  %s %s [%lldM params, %.0fMB, %s%s]\n",
               m->loaded ? "*" : " ",
               m->name,
               (long long)(m->param_count / 1000000),
               m->runtime_mb,
               m->fits_in_ram ? "fits" : "NO FIT",
               m->can_embed ? ", embed" : "");
        printf("    %s\n", m->path);
    }
    const SartreModelProfile *best = sartre_model_best();
    printf("  best: %s\n\n", best ? best->name : "(none)");
}

/* ═══════════════════════════════════════════════════════════════════
 * DEBUG / MONITORING — sartre observes itself
 * ═══════════════════════════════════════════════════════════════════ */

void sartre_print_state(void) {
    if (!sartre_ready) {
        printf("[sartre] not initialized\n");
        return;
    }

    sartre_sample_load();  /* live cpu/mem before reporting */

    int64_t uptime_s = (int64_t)time(NULL) - sys.boot_time_ms / 1000;

    printf("\n=== SARTRE KERNEL STATE ===\n\n");

    printf("Uptime: %llds | Steps: %lld\n\n", (long long)uptime_s, (long long)sys.step_count);

    printf("Inner World:\n");
    printf("  trauma: %.2f  arousal: %.2f  valence: %.2f\n",
           sys.trauma_level, sys.arousal, sys.valence);
    printf("  coherence: %.2f  prophecy_debt: %.2f  entropy: %.2f\n",
           sys.coherence, sys.prophecy_debt, sys.entropy);
    printf("  warmth: %.2f  flow: %.2f\n\n", sys.warmth, sys.flow);

    printf("Overlay: base=%lldB delta=%lldB writes=%d ratio=%.3f\n\n",
           (long long)sys.overlay.base_size, (long long)sys.overlay.delta_size,
           sys.overlay.overlay_writes, sys.overlay.overlay_ratio);

    printf("Resources: mem_pressure=%.2f cpu=%.2f\n", sys.memory_pressure, sys.cpu_load);
    printf("Tongue: %s (RAM: %lld MB, %s)\n\n",
           sartre_tongue_tier_name(sys.tongue_tier),
           (long long)sys.total_ram_mb,
           sys.tongue_override >= 0 ? "override" : "auto");

    printf("Modules (%d):\n", sys.module_count);
    for (int i = 0; i < sys.module_count; i++) {
        printf("  [%d] %-20s status=%d load=%.2f\n",
               i, sys.modules[i].name, sys.modules[i].status, sys.modules[i].load);
    }

    for (int i = 0; i < sys.ns_count; i++)   /* refresh real-process liveness before reporting */
        if (sys.namespaces[i].spawned) sartre_ns_alive(i);
    printf("\nNamespaces (%d):\n", sys.ns_count);
    for (int i = 0; i < sys.ns_count; i++) {
        printf("  [%d] %-16s %-7s pid=%-6d cpu=%.1f%% mem=%.0fMB %s\n",
               i, sys.namespaces[i].name,
               sys.namespaces[i].spawned ? "(proc)" : "(monad)",
               sys.namespaces[i].pid,
               sys.namespaces[i].cpu_share * 100,
               sys.namespaces[i].mem_limit_mb,
               sys.namespaces[i].active ? "ACTIVE" : "dead");
    }

    printf("\nRecent Events (%d):\n", sys.event_count);
    for (int i = 0; i < sys.event_count; i++) {
        printf("  [%d] %s\n", i, sys.last_events[i]);
    }

    printf("\nFlags: spiral=%d wormhole=%d strange_loop=%d\n\n",
           sys.spiral_detected, sys.wormhole_active, sys.strange_loop);
}

/* ═══════════════════════════════════════════════════════════════════
 * JSON EXPORT — for web UI integration
 * ═══════════════════════════════════════════════════════════════════ */

int sartre_state_to_json(char *buf, int max) {
    if (!sartre_ready) return snprintf(buf, max, "{\"ready\":false}");

    sartre_sample_load();  /* live cpu/mem before reporting */

    int64_t uptime_s = (int64_t)time(NULL) - sys.boot_time_ms / 1000;

    /* count installed packages */
    int installed = 0;
    for (int i = 0; i < sys.pkg_count; i++)
        if (sys.packages[i].installed) installed++;

    /* refresh real-process liveness so the counts are truth, not stale cache */
    for (int i = 0; i < sys.ns_count; i++)
        if (sys.namespaces[i].spawned) sartre_ns_alive(i);

    /* count active + real-process namespaces */
    int active_ns = 0, spawned_ns = 0;
    for (int i = 0; i < sys.ns_count; i++) {
        if (sys.namespaces[i].active)  active_ns++;
        if (sys.namespaces[i].spawned) spawned_ns++;
    }

    return snprintf(buf, max,
        "{"
        "\"ready\":true,"
        "\"uptime\":%lld,"
        "\"steps\":%lld,"
        "\"ram_mb\":%lld,"
        "\"tongue\":\"%s\","
        "\"modules\":%d,"
        "\"trauma\":%.3f,"
        "\"arousal\":%.3f,"
        "\"valence\":%.3f,"
        "\"coherence\":%.3f,"
        "\"prophecy_debt\":%.3f,"
        "\"entropy\":%.3f,"
        "\"warmth\":%.3f,"
        "\"flow\":%.3f,"
        "\"memory_field_score\":%.3f,"
        "\"memory_field_prophecy\":%.3f,"
        "\"memory_field_step\":%.3f,"
        "\"schumann_coherence\":%.3f,"
        "\"schumann_phase\":%.3f,"
        "\"overlay_ratio\":%.4f,"
        "\"overlay_writes\":%d,"
        "\"overlay_base\":%lld,"
        "\"overlay_delta\":%lld,"
        "\"namespaces\":%d,"
        "\"ns_active\":%d,"
        "\"ns_spawned\":%d,"
        "\"packages\":%d,"
        "\"pkg_installed\":%d,"
        "\"events\":%d,"
        "\"mem_pressure\":%.3f,"
        "\"cpu_load\":%.3f,"
        "\"spiral\":%d,"
        "\"wormhole\":%d,"
        "\"strange_loop\":%d"
        "}",
        (long long)uptime_s,
        (long long)sys.step_count,
        (long long)sys.total_ram_mb,
        sartre_tongue_tier_name(sys.tongue_tier),
        sys.module_count,
        sys.trauma_level, sys.arousal, sys.valence,
        sys.coherence, sys.prophecy_debt, sys.entropy,
        sys.warmth, sys.flow,
        sys.memory_field_score, sys.memory_field_prophecy, sys.memory_field_step,
        sys.schumann_coherence, sys.schumann_phase,
        sys.overlay.overlay_ratio, sys.overlay.overlay_writes,
        (long long)sys.overlay.base_size, (long long)sys.overlay.delta_size,
        sys.ns_count, active_ns, spawned_ns,
        sys.pkg_count, installed,
        sys.event_count,
        sys.memory_pressure, sys.cpu_load,
        sys.spiral_detected, sys.wormhole_active, sys.strange_loop
    );
}

/* ═══════════════════════════════════════════════════════════════════
 * STANDALONE MAIN — when sartre_kernel.c compiles alone
 *
 * cc sartre_kernel.c -O2 -lm -o sartre_kernel && ./sartre_kernel
 * ═══════════════════════════════════════════════════════════════════ */

#ifndef HAS_DARIO

int main(int argc, char **argv) {
    /* banner to stderr — stdout stays protocol-clean (metrics/pipe emit JSON there) */
    fprintf(stderr, "\n  sartre_kernel — Meta-Linux for the Dario Equation\n");
    fprintf(stderr, "  \"L'existence precede l'essence.\"\n\n");

    sartre_init(NULL);

    /* hold mode: spawn a real sleeper, print its pid, block — so an external
     * `ps -p <pid>` can confirm the slot is a real OS process. Press Enter to
     * terminate+reap and exit (external ps then confirms it is gone, no zombie). */
    if (argc > 1 && strcmp(argv[1], "hold") == 0) {
        char *targv[] = { "/bin/sh", "-c", "sleep 30", NULL };
        int h = sartre_ns_spawn("hold_sleeper", targv, 64.0f);
        if (h < 0) { printf("SPAWN_FAILED\n"); return 1; }
        printf("SPAWNED_PID=%d alive=%d\n", sartre_ns_get(h)->pid, sartre_ns_alive(h));
        fflush(stdout);
        getchar();
        sartre_ns_kill(h);
        printf("KILLED pid=%d alive=%d\n", sartre_ns_get(h)->pid, sartre_ns_alive(h));
        sartre_shutdown();
        return 0;
    }

    /* pipe mode: spawn ANY utility into a piped slot and read its stdout — proving
     * the slot is language-agnostic (Rust repo_monitor, C context_processor, ...).
     * usage: sartre_kernel pipe <utility-binary> [args...] */
    if (argc > 2 && strcmp(argv[1], "pipe") == 0) {
        char *const *uargv = &argv[2];          /* binary + args, NULL-terminated by the OS */
        const char *slash = strrchr(argv[2], '/');
        const char *uname = slash ? slash + 1 : argv[2];
        int fd = -1;
        int id = sartre_ns_spawn_piped(uname, uargv, 0.0f, &fd);
        if (id < 0) { printf("PIPE_SPAWN_FAILED\n"); return 1; }
        printf("[pipe] slot[%d] pid=%d reading events:\n", id, sartre_ns_get(id)->pid);
        char evbuf[8192];
        int total = 0;
        for (;;) {                          /* --once: data then EOF */
            ssize_t n = read(fd, evbuf + total, sizeof(evbuf) - 1 - total);
            if (n > 0) {
                total += (int)n;
                if (total >= (int)sizeof(evbuf) - 1) break;
            } else if (n < 0 && errno == EINTR) {
                continue;                   /* interrupted: retry, don't truncate */
            } else {
                break;                      /* EOF (0) or real error */
            }
        }
        evbuf[total] = '\0';
        close(fd);
        fputs(evbuf, stdout);
        sartre_ns_kill(id);  /* preflight-reaps the already-exited child */
        printf("[pipe] read %d bytes from slot, alive=%d\n", total, sartre_ns_alive(id));
#ifdef HAS_PERCEPTION
        {   /* close the loop: utility events -> AML field pressure */
            SartrePerception perc;
            sartre_perceive_from_events(evbuf, &perc);
            char aml[256];
            sartre_perceive_to_aml(&perc, aml, sizeof aml);
            printf("[perception] changed=%d readme=%d -> AML:\n%s\n",
                   perc.changed, perc.readme_changed, aml);
        }
#endif
        sartre_shutdown();
        return 0;
    }

    /* metrics mode: the live telemetry hub — sample real cpu/mem and print the
     * state as JSON. Optional argv[2] = a field-weather JSON to ingest first
     * (the reciprocal seam): sartre_kernel metrics '{"debt":2.0,"coherence":0.8}'.
     * With `--stream`: the field->SARTRE transport — the dock pipes field-weather
     * JSON lines on stdin, SARTRE ingests each and emits the refreshed hub state on
     * stdout (the reverse of how the dock reads a utility's stdout). */
    if (argc > 1 && strcmp(argv[1], "metrics") == 0) {
        /* sartre_init already ran at the top of main — do not re-init */
        if (argc > 2 && strcmp(argv[2], "--stream") == 0) {
            signal(SIGPIPE, SIG_IGN);   /* reader may close; exit cleanly, not by signal */
            char line[4096], json[2048];
            while (fgets(line, sizeof line, stdin)) {
                size_t len = strlen(line);
                if (len == sizeof(line) - 1 && line[len - 1] != '\n') {
                    /* record longer than the frame: drain the rest and skip it, so a
                     * truncated fragment is never ingested as its own message */
                    int c;
                    while ((c = getchar()) != '\n' && c != EOF) { }
                    continue;
                }
                sartre_ingest_metrics_json(line);
                sartre_state_to_json(json, sizeof json);   /* refreshes live cpu/mem */
                if (printf("%s\n", json) < 0 || fflush(stdout) != 0) break;  /* reader gone */
            }
            sartre_shutdown();
            return 0;
        }
        if (argc > 2) sartre_ingest_metrics_json(argv[2]);
        char json[2048];
        sartre_state_to_json(json, sizeof json);  /* refreshes live cpu/mem */
        printf("%s\n", json);
        sartre_shutdown();
        return 0;
    }

    /* register core packages */
    sartre_pkg_register("dario_equation", "1.0.0", 83);
    sartre_pkg_register("hebbian_field",  "1.0.0", 12);
    sartre_pkg_register("prophecy",       "1.0.0", 8);
    sartre_pkg_register("trauma_engine",  "1.0.0", 4);
    sartre_pkg_register("velocity_ops",   "1.0.0", 6);
    sartre_pkg_register("chambers",       "1.0.0", 10);
    sartre_pkg_register("overlay_fs",     "1.0.0", 2);

    /* install core */
    sartre_pkg_install("dario_equation");
    sartre_pkg_install("hebbian_field");
    sartre_pkg_install("prophecy");

    /* init overlay */
    sartre_overlay_init(83 * 1024); /* dario.c ~ 83KB */

    /* create a namespace for the equation */
    sartre_ns_create("dario", 0.8f, 64.0f);
    sartre_ns_create("observer", 0.1f, 8.0f);

    /* === real process-slot smoke (brick #1) — self-verifying lifecycle === */
    {
        int pass = 0, total = 5;

        char *sargv[] = { "/bin/sh", "-c", "sleep 30", NULL };
        int s = sartre_ns_spawn("smoke_sleeper", sargv, 64.0f);
        int alive1 = (s >= 0) && sartre_ns_alive(s);
        printf("[smoke] spawn sleep -> alive=%d  %s\n", alive1, alive1 ? "PASS" : "FAIL");
        pass += alive1;

        if (s >= 0) sartre_ns_kill(s);
        int dead1 = (s >= 0) && !sartre_ns_alive(s);
        printf("[smoke] kill      -> alive=%d  %s\n", s >= 0 ? sartre_ns_alive(s) : -1,
               dead1 ? "PASS" : "FAIL");
        pass += dead1;

        char *eargv[] = { "/bin/sh", "-c", "exit 0", NULL };
        int e = sartre_ns_spawn("smoke_exiter", eargv, 64.0f);
        struct timespec ts = { 0, 100 * 1000 * 1000 };  /* 100ms for the child to exit */
        nanosleep(&ts, NULL);
        int reaped = (e >= 0) && !sartre_ns_alive(e);
        printf("[smoke] self-exit -> alive=%d  %s\n", e >= 0 ? sartre_ns_alive(e) : -1,
               reaped ? "PASS" : "FAIL");
        pass += reaped;

        char *rargv[] = { "/bin/sh", "-c", "exit 0", NULL };
        int r1 = sartre_ns_spawn("smoke_reuse_exiter", rargv, 64.0f);
        nanosleep(&ts, NULL);                /* do NOT call alive(r1); spawn must reap it */
        int count_before = sys.ns_count;
        int r2 = sartre_ns_spawn("smoke_reuse_next", rargv, 64.0f);
        int reused = (r1 >= 0 && r2 == r1 && sys.ns_count == count_before);
        nanosleep(&ts, NULL);
        if (r2 >= 0) (void)sartre_ns_alive(r2);
        printf("[smoke] reuse     -> slot=%d/%d count=%d  %s\n", r1, r2, sys.ns_count,
               reused ? "PASS" : "FAIL");
        pass += reused;

        char *dargv[] = { "/bin/sh", "-c", "sleep 30", NULL };
        int d = sartre_ns_spawn("smoke_destroy", dargv, 64.0f);
        if (d >= 0) sartre_ns_destroy(d);     /* spawned destroy must terminate+reap, not leak */
        int dead2 = (d >= 0) && !sartre_ns_alive(d);
        printf("[smoke] destroy   -> alive=%d  %s\n", d >= 0 ? sartre_ns_alive(d) : -1,
               dead2 ? "PASS" : "FAIL");
        pass += dead2;

        printf("[smoke] %d/%d PASS\n", pass, total);
    }

    /* simulate some inner state */
    sartre_update_inner_state(0.15f, 0.6f, 0.7f, 0.85f, 1.2f);
    sartre_notify_event("boot_complete");

    /* print state */
    sartre_print_state();

    /* JSON export demo */
    char json[2048];
    sartre_state_to_json(json, sizeof(json));
    printf("JSON:\n%s\n\n", json);

    /* package list */
    sartre_pkg_list();

    /* shutdown-reap check: leave a live spawned child — sartre_shutdown must reap it
     * (external `ps -p <LEFT_PID>` after exit confirms no orphan/zombie). */
    {
        char *largv[] = { "/bin/sh", "-c", "sleep 30", NULL };
        int l = sartre_ns_spawn("shutdown_left", largv, 64.0f);
        if (l >= 0) printf("LEFT_PID=%d (expect reaped by shutdown)\n", sartre_ns_get(l)->pid);
    }

    sartre_shutdown();
    return 0;
}

#endif /* !HAS_DARIO */
