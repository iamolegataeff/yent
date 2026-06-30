# feeling.jl — Yent's feeling mathematics, run in-process on real Julia (libjulia embed).
#
# These are the HighMathEngine formulas (arianna.c legacy inner_world/high.go, ancestor
# nicole/high.py — the Julia math brain), ported to Julia and executed by the Julia runtime
# via jl_eval_string. NOT a Go re-implementation: the Julia VM computes these. The Go side
# (innerworld/feeling cgo) loads this file once on jl_init and calls these functions on the
# circle text.

module Feeling

export char_entropy, perplexity, semantic_distance, ngram_overlap

# CharEntropy — Shannon entropy (bits) of the CHARACTER distribution (legacy CharEntropy).
function char_entropy(text::String)::Float64
    isempty(text) && return 0.0
    counts = Dict{Char,Int}()
    total = 0
    for c in text
        counts[c] = get(counts, c, 0) + 1
        total += 1
    end
    h = 0.0
    for (_, cnt) in counts
        p = cnt / total
        p > 0 && (h -= p * log2(p))
    end
    return h
end

# Perplexity — bigram perplexity over characters (legacy Perplexity).
function perplexity(text::String)::Float64
    rs = collect(text)
    length(rs) < 2 && return 1.0
    bigram = Dict{Tuple{Char,Char},Int}()
    unigram = Dict{Char,Int}()
    for i in 1:length(rs)-1
        k = (rs[i], rs[i+1])
        bigram[k] = get(bigram, k, 0) + 1
        unigram[rs[i]] = get(unigram, rs[i], 0) + 1
    end
    unigram[rs[end]] = get(unigram, rs[end], 0) + 1
    logp = 0.0
    n = 0
    for i in 1:length(rs)-1
        bc = get(bigram, (rs[i], rs[i+1]), 0)
        uc = get(unigram, rs[i], 0)
        if uc > 0 && bc > 0
            logp += log2(bc / uc)
            n += 1
        end
    end
    n == 0 && return 1.0
    return 2.0 ^ (-logp / n)
end

_words(t::String) = split(lowercase(t))

# SemanticDistance — 1 - bag-of-words cosine similarity (legacy SemanticDistance).
function semantic_distance(a::String, b::String)::Float64
    wa, wb = _words(a), _words(b)
    (isempty(wa) || isempty(wb)) && return 1.0
    vocab = Dict{String,Int}()
    for w in Iterators.flatten((wa, wb))
        haskey(vocab, w) || (vocab[w] = length(vocab) + 1)
    end
    va = zeros(Float64, length(vocab))
    vb = zeros(Float64, length(vocab))
    for w in wa; va[vocab[w]] += 1; end
    for w in wb; vb[vocab[w]] += 1; end
    dot = sum(va .* vb)
    na = sqrt(sum(va .^ 2))
    nb = sqrt(sum(vb .^ 2))
    (na == 0 || nb == 0) && return 1.0
    return 1.0 - dot / (na * nb)
end

# NgramOverlap — Jaccard overlap of character n-grams (legacy NgramOverlap).
function ngram_overlap(a::String, b::String, n::Int)::Float64
    grams(t) = Set(String(collect(t)[i:i+n-1]) for i in 1:max(0, length(collect(t)) - n + 1))
    ga, gb = grams(a), grams(b)
    (isempty(ga) || isempty(gb)) && return 0.0
    inter = length(intersect(ga, gb))
    uni = length(union(ga, gb))
    uni == 0 ? 0.0 : inter / uni
end

end # module
