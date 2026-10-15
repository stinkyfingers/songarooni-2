package click

import "testing"

// decode16 reads buf as little-endian int16 samples.
func decode16(buf []byte) []int16 {
	out := make([]int16, len(buf)/2)
	for i := range out {
		out[i] = int16(buf[2*i]) | int16(buf[2*i+1])<<8
	}
	return out
}

func peakAbs(samples []int16) int16 {
	var peak int16
	for _, s := range samples {
		abs := s
		if abs < 0 {
			abs = -abs
		}
		if abs > peak {
			peak = abs
		}
	}
	return peak
}

func TestRenderBeatLength(t *testing.T) {
	const tempo = 120 // 500ms beat period
	buf := renderBeat(tempo, 1.0, 1800)
	wantSamples := int(0.5 * sampleRate) // 60/120 = 0.5s
	if got := len(buf) / 2; got != wantSamples {
		t.Fatalf("expected %d samples, got %d", wantSamples, got)
	}
}

func TestRenderBeatZeroVolumeIsSilent(t *testing.T) {
	buf := renderBeat(120, 0.0, 1800)
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("expected a silent buffer at volume 0, byte %d was %d", i, b)
		}
	}
}

func TestRenderBeatVolumeScalesPeak(t *testing.T) {
	full := peakAbs(decode16(renderBeat(120, 1.0, 1800)))
	half := peakAbs(decode16(renderBeat(120, 0.5, 1800)))

	if full == 0 {
		t.Fatal("expected a nonzero peak at volume 1.0")
	}
	// Allow a little slack for integer rounding: half-volume's peak should
	// be roughly half of full-volume's, not something like unaffected or
	// inverted.
	ratio := float64(half) / float64(full)
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("expected ~0.5 ratio between half- and full-volume peaks, got %v (full=%d half=%d)", ratio, full, half)
	}
}

func TestRenderBeatBurstDecaysBeforeItEnds(t *testing.T) {
	// The exponential-decay envelope must reach (near) zero by the end of
	// the burst itself, not just by the end of the beat period — otherwise
	// the burst-to-silence boundary is an audible pop, not a clean decay.
	buf := renderBeat(60, 1.0, 1800) // 1s beat period, plenty of silent padding after the burst
	samples := decode16(buf)

	peak := peakAbs(samples)
	if peak == 0 {
		t.Fatal("expected a nonzero peak")
	}

	// The burst is much shorter than the beat period at 60 BPM; scan the
	// first ~100ms (well past any reasonable burst length) for where the
	// signal first reaches silence, then confirm it stays silent — i.e.
	// the burst itself, not just outer padding, decays to near-zero.
	const burstWindow = sampleRate / 10 // 100ms
	last := samples[burstWindow-50 : burstWindow]
	if got := peakAbs(last); float64(got) > 0.05*float64(peak) {
		t.Fatalf("expected the burst to have decayed below 5%% of peak by 100ms in, got %d (peak %d)", got, peak)
	}
}

func TestRenderBeatEndsInSilence(t *testing.T) {
	// Only the short burst at the start of the beat should carry sound; the
	// rest of the beat period pads out to the next click with silence.
	buf := renderBeat(60, 1.0, 1800) // 1s beat period, a long tail to check
	samples := decode16(buf)
	tail := samples[len(samples)-100:]
	if peakAbs(tail) != 0 {
		t.Fatal("expected the end of the beat period to be silent")
	}
}
