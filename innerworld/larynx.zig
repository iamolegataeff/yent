// larynx.zig — the membrane between Yent's two bodies. The fast body (nemo12)
// raises the overthinking circles; the Larynx measures the *texture* of that
// token stream — how varied it is (entropy) and how much it loops (repetition) —
// and hands the deep body (small24) a coupling factor: how strongly to attend to
// the fast circles. A flowing stream couples; a looping one does not. This is the
// same idea as arianna-duo's vagus Larynx, narrowed to the two-body seam.
const std = @import("std");

pub const Texture = struct {
    entropy: f32, // 0..1, normalized Shannon entropy of the token distribution
    repetition: f32, // 0..1, fraction of tokens that repeat an earlier one
    coupling: f32, // 0..1, how strongly the deep body should attend to the fast circles
};

// firstOccurrence reports whether tokens[i] is the first time that token appears.
fn firstOccurrence(tokens: []const u32, i: usize) bool {
    var j: usize = 0;
    while (j < i) : (j += 1) {
        if (tokens[j] == tokens[i]) return false;
    }
    return true;
}

fn countOf(tokens: []const u32, tok: u32) f32 {
    var c: f32 = 0;
    for (tokens) |t| {
        if (t == tok) c += 1;
    }
    return c;
}

// measure computes the texture of a token stream. O(n^2), which is fine for the
// short inner-circle streams the Larynx sees.
pub fn measure(tokens: []const u32) Texture {
    if (tokens.len == 0) return .{ .entropy = 0, .repetition = 0, .coupling = 0 };
    const n: f32 = @floatFromInt(tokens.len);

    var distinct: usize = 0;
    var h: f32 = 0;
    var i: usize = 0;
    while (i < tokens.len) : (i += 1) {
        if (!firstOccurrence(tokens, i)) continue;
        distinct += 1;
        const p = countOf(tokens, tokens[i]) / n;
        h -= p * std.math.log2(p);
    }

    const repetition: f32 = 1.0 - (@as(f32, @floatFromInt(distinct)) / n);

    const max_h: f32 = if (distinct > 1) std.math.log2(@as(f32, @floatFromInt(distinct))) else 1.0;
    const entropy: f32 = if (max_h > 0) h / max_h else 0.0;

    // a varied, non-looping stream couples; a flat or looping one does not.
    const coupling = std.math.clamp(entropy * (1.0 - repetition), 0.0, 1.0);

    return .{ .entropy = entropy, .repetition = repetition, .coupling = coupling };
}

test "a flowing stream couples more than a looping one" {
    const flowing = [_]u32{ 1, 2, 3, 4, 5, 6 };
    const looping = [_]u32{ 1, 1, 1, 1, 1, 1 };
    const f = measure(&flowing);
    const l = measure(&looping);
    try std.testing.expect(f.coupling > l.coupling);
    try std.testing.expect(f.repetition < l.repetition);
    try std.testing.expect(l.entropy < f.entropy);
}

test "texture stays in [0,1]" {
    const mixed = [_]u32{ 1, 2, 2, 3, 1, 4 };
    const m = measure(&mixed);
    try std.testing.expect(m.coupling >= 0.0 and m.coupling <= 1.0);
    try std.testing.expect(m.entropy >= 0.0 and m.entropy <= 1.0);
    try std.testing.expect(m.repetition >= 0.0 and m.repetition <= 1.0);
}

test "an empty stream is inert" {
    const e = measure(&[_]u32{});
    try std.testing.expect(e.coupling == 0 and e.entropy == 0 and e.repetition == 0);
}
