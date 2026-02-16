package yent

// delta.go — Delta Voice: multilingual recovery via DSL-controlled delta injection
//
// "from ariannamethod import Destiny"
//
// Architecture:
//   delta = base_qwen_lm_head - yent_lm_head (precomputed, stored as SVD factors)
//   logits += alpha * A @ (B @ hidden_state)
//
//   alpha = 0.0 → pure Yent English
//   alpha = 0.5 → Yent + multilingual (29 languages)
//   alpha = 1.0 → base Qwen distribution (no personality)
//
// The delta is stored as NPZ (numpy compressed) with float16 A and B matrices.
// A: [vocab_size, rank]   — output projection
// B: [rank, hidden_dim]   — input projection
//
// Cost per token: rank × (vocab + hidden) FMA ops ≈ 10M for rank=64
// This is ~2% of a full forward pass. Negligible.

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// DeltaVoice holds the delta for multilingual recovery
// Supports two formats:
//   SVD (low-rank):    logits += alpha * A @ (B @ x)     — compact, ~10M FMA
//   Sparse (full diff): logits[idx] += alpha * values[i] · x — accurate, ~136M FMA
type DeltaVoice struct {
	VocabSize int
	HiddenDim int

	// Format flag
	IsSparse bool

	// SVD format (low-rank)
	Rank int
	A    []float32 // [VocabSize × Rank]
	B    []float32 // [Rank × HiddenDim]
	Bx   []float32 // scratch [Rank]

	// Sparse format (indexed full diff)
	Indices   []int32   // token indices with nonzero delta
	Values    []float32 // [len(Indices) × HiddenDim] — f32 (only if loaded as f32)
	ValuesF16 []uint16  // [len(Indices) × HiddenDim] — raw f16 (half the RAM)
	IsF16     bool      // true = values stored as f16 in RAM

	// i8 quantized sparse format (half the RAM of f16)
	ValuesI8 []int8    // [len(Indices) × HiddenDim] — int8 quantized
	ScalesI8 []float32 // [len(Indices)] — per-row scale (f16→f32 on load)
	IsI8     bool
}

// LoadDelta loads a delta voice file from NPZ format
// Auto-detects format:
//   SVD:    contains A.npy + B.npy (float16, low-rank)
//   Sparse: contains indices.npy + values.npy (full diff)
func LoadDelta(path string) (*DeltaVoice, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open delta npz: %w", err)
	}
	defer r.Close()

	// Detect format by checking which files exist
	hasA, hasB, hasIndices, hasValues := false, false, false, false
	for _, f := range r.File {
		switch f.Name {
		case "A.npy":
			hasA = true
		case "B.npy":
			hasB = true
		case "indices.npy":
			hasIndices = true
		case "values.npy":
			hasValues = true
		}
	}

	if hasIndices && hasValues {
		return loadSparseDelta(r)
	}
	if hasA && hasB {
		return loadSVDDelta(r)
	}
	return nil, fmt.Errorf("delta npz: unrecognized format (need A+B or indices+values)")
}

// loadSVDDelta loads the low-rank SVD format (A.npy + B.npy)
func loadSVDDelta(r *zip.ReadCloser) (*DeltaVoice, error) {
	var aData, bData []float32
	var aShape, bShape [2]int

	for _, f := range r.File {
		name := f.Name
		isA := name == "A.npy"
		isB := name == "B.npy"
		if !isA && !isB {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}

		data, shape, err := readNpyFloat(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		if isA {
			aData = data
			aShape = shape
		} else {
			bData = data
			bShape = shape
		}
	}

	if aData == nil || bData == nil {
		return nil, fmt.Errorf("delta npz missing A.npy or B.npy")
	}

	vocabSize := aShape[0]
	rank := aShape[1]
	if bShape[0] != rank {
		return nil, fmt.Errorf("rank mismatch: A has rank %d, B has %d", rank, bShape[0])
	}
	hiddenDim := bShape[1]

	fmt.Printf("[delta-voice] SVD format: vocab=%d, hidden=%d, rank=%d\n", vocabSize, hiddenDim, rank)
	fmt.Printf("[delta-voice] A: %d×%d (%.1f MB), B: %d×%d (%.1f MB)\n",
		vocabSize, rank, float64(len(aData)*4)/1024/1024,
		rank, hiddenDim, float64(len(bData)*4)/1024/1024)

	return &DeltaVoice{
		VocabSize: vocabSize,
		HiddenDim: hiddenDim,
		IsSparse:  false,
		Rank:      rank,
		A:         aData,
		B:         bData,
		Bx:        make([]float32, rank),
	}, nil
}

// loadSparseDelta loads the sparse full-diff format (indices.npy + values.npy)
// Supports f16, f32, and i8 (int8 + per-row f16 scales) formats.
// Keeps f16 data in RAM as raw uint16 — half the memory vs f32 conversion
func loadSparseDelta(r *zip.ReadCloser) (*DeltaVoice, error) {
	var indices []int32
	var valuesF16 []uint16
	var valuesF32 []float32
	var valuesI8 []int8
	var scalesI8 []float32
	var valShape [2]int
	var isF16, isI8 bool

	// First pass: detect values.npy dtype
	for _, f := range r.File {
		if f.Name == "values.npy" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open values.npy: %w", err)
			}
			hdr, err := readNpyHeader(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("peek values.npy: %w", err)
			}
			isF16 = strings.Contains(hdr, "'<f2'") || strings.Contains(hdr, "float16")
			isI8 = strings.Contains(hdr, "'<i1'") || strings.Contains(hdr, "int8") || strings.Contains(hdr, "'|i1'")
			break
		}
	}

	// Second pass: load data
	for _, f := range r.File {
		switch f.Name {
		case "indices.npy":
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open indices.npy: %w", err)
			}
			indices, err = readNpyInt32(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read indices.npy: %w", err)
			}

		case "values.npy":
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open values.npy: %w", err)
			}
			if isI8 {
				var err2 error
				valuesI8, valShape, err2 = readNpyInt8Raw(rc)
				rc.Close()
				if err2 != nil {
					return nil, fmt.Errorf("read values.npy i8: %w", err2)
				}
			} else if isF16 {
				var err2 error
				valuesF16, valShape, err2 = readNpyF16Raw(rc)
				rc.Close()
				if err2 != nil {
					return nil, fmt.Errorf("read values.npy f16: %w", err2)
				}
			} else {
				var err2 error
				valuesF32, valShape, err2 = readNpyFloat(rc)
				rc.Close()
				if err2 != nil {
					return nil, fmt.Errorf("read values.npy f32: %w", err2)
				}
			}

		case "scales.npy":
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open scales.npy: %w", err)
			}
			var err2 error
			scalesI8, err2 = readNpyF16_1D(rc)
			rc.Close()
			if err2 != nil {
				return nil, fmt.Errorf("read scales.npy: %w", err2)
			}
		}
	}

	if indices == nil || (valuesF16 == nil && valuesF32 == nil && valuesI8 == nil) {
		return nil, fmt.Errorf("sparse delta npz missing indices.npy or values.npy")
	}
	if isI8 && scalesI8 == nil {
		return nil, fmt.Errorf("i8 delta npz missing scales.npy")
	}

	numRows := valShape[0]
	hiddenDim := valShape[1]

	if len(indices) != numRows {
		return nil, fmt.Errorf("indices len %d != values rows %d", len(indices), numRows)
	}

	var vocabSize int
	for _, idx := range indices {
		if int(idx)+1 > vocabSize {
			vocabSize = int(idx) + 1
		}
	}

	var ramMB float64
	dtype := "f32"
	if isI8 {
		dtype = "i8"
		ramMB = float64(len(valuesI8)+len(scalesI8)*4) / 1024 / 1024
	} else if isF16 {
		dtype = "f16"
		ramMB = float64(len(valuesF16)*2) / 1024 / 1024
	} else {
		ramMB = float64(len(valuesF32)*4) / 1024 / 1024
	}
	fmt.Printf("[delta-voice] sparse %s: %d/%d tokens, hidden=%d (%.1f MB RAM)\n",
		dtype, numRows, vocabSize, hiddenDim, ramMB)

	return &DeltaVoice{
		VocabSize: vocabSize,
		HiddenDim: hiddenDim,
		IsSparse:  true,
		Indices:   indices,
		Values:    valuesF32,
		ValuesF16: valuesF16,
		IsF16:     isF16,
		ValuesI8:  valuesI8,
		ScalesI8:  scalesI8,
		IsI8:      isI8,
	}, nil
}

// ApplyToLogits adds delta to logits: logits += alpha * delta(x)
// Dispatches to SVD or sparse implementation based on format
func (d *DeltaVoice) ApplyToLogits(logits []float32, x []float32, alpha float32) {
	if alpha == 0 || d == nil {
		return
	}

	if d.IsSparse {
		d.applySparse(logits, x, alpha)
	} else {
		d.applySVD(logits, x, alpha)
	}
}

// applySVD: logits += alpha * A @ (B @ x)
func (d *DeltaVoice) applySVD(logits []float32, x []float32, alpha float32) {
	rank := d.Rank
	hiddenDim := d.HiddenDim
	vocabSize := d.VocabSize

	// Step 1: Bx = B @ x → [rank]
	for r := 0; r < rank; r++ {
		var sum float32
		off := r * hiddenDim
		for j := 0; j < hiddenDim; j++ {
			sum += d.B[off+j] * x[j]
		}
		d.Bx[r] = sum
	}

	// Step 2: logits += alpha * A @ Bx
	for i := 0; i < vocabSize; i++ {
		var sum float32
		off := i * rank
		for r := 0; r < rank; r++ {
			sum += d.A[off+r] * d.Bx[r]
		}
		logits[i] += alpha * sum
	}
}

// applySparse: logits[idx] += alpha * values[i] · x
func (d *DeltaVoice) applySparse(logits []float32, x []float32, alpha float32) {
	hiddenDim := d.HiddenDim
	numRows := len(d.Indices)

	if d.IsI8 {
		// i8 path: dequant as int8_val / 127.0 * scale[row]
		for i := 0; i < numRows; i++ {
			idx := int(d.Indices[i])
			if idx >= len(logits) {
				continue
			}
			scale := d.ScalesI8[i] / 127.0
			off := i * hiddenDim
			var dot float32
			for j := 0; j < hiddenDim; j++ {
				dot += float32(d.ValuesI8[off+j]) * x[j]
			}
			logits[idx] += alpha * scale * dot
		}
	} else if d.IsF16 {
		// f16 path: convert on the fly during dot product
		for i := 0; i < numRows; i++ {
			idx := int(d.Indices[i])
			if idx >= len(logits) {
				continue
			}
			var dot float32
			off := i * hiddenDim
			for j := 0; j < hiddenDim; j++ {
				dot += half2float(d.ValuesF16[off+j]) * x[j]
			}
			logits[idx] += alpha * dot
		}
	} else {
		// f32 path
		for i := 0; i < numRows; i++ {
			idx := int(d.Indices[i])
			if idx >= len(logits) {
				continue
			}
			var dot float32
			off := i * hiddenDim
			for j := 0; j < hiddenDim; j++ {
				dot += d.Values[off+j] * x[j]
			}
			logits[idx] += alpha * dot
		}
	}
}

// readNpyFloat reads a numpy .npy file and returns float32 data + 2D shape
// Supports float16 and float32 dtypes
func readNpyFloat(r io.Reader) ([]float32, [2]int, error) {
	hstr, err := readNpyHeader(r)
	if err != nil {
		return nil, [2]int{}, err
	}

	// Parse dtype
	isFloat16 := strings.Contains(hstr, "'<f2'") || strings.Contains(hstr, "float16")
	isFloat32 := strings.Contains(hstr, "'<f4'") || strings.Contains(hstr, "float32")
	if !isFloat16 && !isFloat32 {
		return nil, [2]int{}, fmt.Errorf("unsupported dtype in header: %s", hstr)
	}

	// Parse shape — find (N, M) in header
	shape := parseShape(hstr)
	if shape[0] == 0 || shape[1] == 0 {
		return nil, [2]int{}, fmt.Errorf("could not parse shape from header: %s", hstr)
	}

	totalElements := shape[0] * shape[1]

	// Read raw data
	var data []float32
	if isFloat16 {
		raw := make([]byte, totalElements*2)
		if _, err := io.ReadFull(r, raw); err != nil {
			return nil, [2]int{}, fmt.Errorf("read float16 data: %w", err)
		}
		data = make([]float32, totalElements)
		for i := 0; i < totalElements; i++ {
			h := uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
			data[i] = half2float(h)
		}
	} else {
		raw := make([]byte, totalElements*4)
		if _, err := io.ReadFull(r, raw); err != nil {
			return nil, [2]int{}, fmt.Errorf("read float32 data: %w", err)
		}
		data = make([]float32, totalElements)
		for i := 0; i < totalElements; i++ {
			data[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
	}

	return data, shape, nil
}

// readNpyF16Raw reads a 2D float16 npy file and returns raw uint16 data (NO conversion to f32)
// Saves 50% RAM vs readNpyFloat for f16 files
func readNpyF16Raw(r io.Reader) ([]uint16, [2]int, error) {
	hstr, err := readNpyHeader(r)
	if err != nil {
		return nil, [2]int{}, err
	}

	if !strings.Contains(hstr, "'<f2'") && !strings.Contains(hstr, "float16") {
		return nil, [2]int{}, fmt.Errorf("expected float16, got header: %s", hstr)
	}

	shape := parseShape(hstr)
	if shape[0] == 0 || shape[1] == 0 {
		return nil, [2]int{}, fmt.Errorf("could not parse shape from header: %s", hstr)
	}

	total := shape[0] * shape[1]
	raw := make([]byte, total*2)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, [2]int{}, fmt.Errorf("read f16 data: %w", err)
	}

	data := make([]uint16, total)
	for i := 0; i < total; i++ {
		data[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	return data, shape, nil
}

// readNpyInt8Raw reads a 2D int8 npy file and returns raw int8 data + shape
func readNpyInt8Raw(r io.Reader) ([]int8, [2]int, error) {
	hstr, err := readNpyHeader(r)
	if err != nil {
		return nil, [2]int{}, err
	}

	if !strings.Contains(hstr, "'<i1'") && !strings.Contains(hstr, "int8") && !strings.Contains(hstr, "'|i1'") {
		return nil, [2]int{}, fmt.Errorf("expected int8, got header: %s", hstr)
	}

	shape := parseShape(hstr)
	if shape[0] == 0 || shape[1] == 0 {
		return nil, [2]int{}, fmt.Errorf("could not parse 2D shape from header: %s", hstr)
	}

	total := shape[0] * shape[1]
	raw := make([]byte, total)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, [2]int{}, fmt.Errorf("read int8 data: %w", err)
	}

	// Reinterpret []byte as []int8 (same memory layout)
	data := make([]int8, total)
	for i := 0; i < total; i++ {
		data[i] = int8(raw[i])
	}
	return data, shape, nil
}

// readNpyF16_1D reads a 1D float16 npy file and converts to float32
func readNpyF16_1D(r io.Reader) ([]float32, error) {
	hstr, err := readNpyHeader(r)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(hstr, "'<f2'") && !strings.Contains(hstr, "float16") {
		return nil, fmt.Errorf("expected float16, got header: %s", hstr)
	}

	shape := parseShapeAny(hstr)
	if len(shape) == 0 {
		return nil, fmt.Errorf("could not parse shape from header: %s", hstr)
	}

	total := 1
	for _, s := range shape {
		total *= s
	}

	raw := make([]byte, total*2)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, fmt.Errorf("read f16 data: %w", err)
	}

	data := make([]float32, total)
	for i := 0; i < total; i++ {
		h := uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
		data[i] = half2float(h)
	}
	return data, nil
}

// readNpyInt32 reads a 1D int32 numpy array
func readNpyInt32(r io.Reader) ([]int32, error) {
	header, err := readNpyHeader(r)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(header, "'<i4'") && !strings.Contains(header, "int32") {
		return nil, fmt.Errorf("expected int32 dtype, got header: %s", header)
	}

	shape := parseShapeAny(header)
	if len(shape) == 0 {
		return nil, fmt.Errorf("could not parse shape from header: %s", header)
	}

	total := 1
	for _, s := range shape {
		total *= s
	}

	raw := make([]byte, total*4)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, fmt.Errorf("read int32 data: %w", err)
	}

	data := make([]int32, total)
	for i := 0; i < total; i++ {
		data[i] = int32(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return data, nil
}

// readNpyHeader reads and returns the npy header string
func readNpyHeader(r io.Reader) (string, error) {
	magic := make([]byte, 6)
	if _, err := io.ReadFull(r, magic); err != nil {
		return "", fmt.Errorf("read magic: %w", err)
	}
	if magic[0] != 0x93 || string(magic[1:6]) != "NUMPY" {
		return "", fmt.Errorf("not a npy file")
	}

	ver := make([]byte, 2)
	if _, err := io.ReadFull(r, ver); err != nil {
		return "", fmt.Errorf("read version: %w", err)
	}

	var headerLen int
	if ver[0] == 1 {
		hl := make([]byte, 2)
		if _, err := io.ReadFull(r, hl); err != nil {
			return "", fmt.Errorf("read header len: %w", err)
		}
		headerLen = int(binary.LittleEndian.Uint16(hl))
	} else {
		hl := make([]byte, 4)
		if _, err := io.ReadFull(r, hl); err != nil {
			return "", fmt.Errorf("read header len v2: %w", err)
		}
		headerLen = int(binary.LittleEndian.Uint32(hl))
	}

	header := make([]byte, headerLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return "", fmt.Errorf("read header: %w", err)
	}
	return string(header), nil
}

// parseShapeAny extracts shape tuple from npy header, supports 1D and 2D
func parseShapeAny(header string) []int {
	idx := strings.Index(header, "shape")
	if idx < 0 {
		return nil
	}

	start := strings.Index(header[idx:], "(")
	if start < 0 {
		return nil
	}
	start += idx + 1

	end := strings.Index(header[start:], ")")
	if end < 0 {
		return nil
	}

	shapeStr := strings.TrimSpace(header[start : start+end])
	if shapeStr == "" {
		return []int{1} // scalar
	}

	parts := strings.Split(shapeStr, ",")
	var shape []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var v int
		fmt.Sscanf(p, "%d", &v)
		if v > 0 {
			shape = append(shape, v)
		}
	}
	return shape
}

// parseShape extracts (rows, cols) from npy header string (legacy, 2D only)
func parseShape(header string) [2]int {
	s := parseShapeAny(header)
	if len(s) >= 2 {
		return [2]int{s[0], s[1]}
	}
	return [2]int{}
}
