package audio

import (
	"fmt"
	"math"
	"math/cmplx"
)

// FrontEndVersion names the exact feature definition so two reports are only
// comparable when they share it.
const FrontEndVersion = "coimnet-logmel/v1"

// FrontEndConfig fixes the analysis: Hann window of Window samples (a power of
// two, >= 16), hop Hop (1..Window), Mels triangular filters (>= 1) on the HTK
// mel scale (m = 2595*log10(1+f/700)) between FMin and FMax Hz
// (0 <= FMin < FMax <= rate/2), log10 of (mel energy + Floor) with Floor > 0.
type FrontEndConfig struct {
	Window int     `json:"window"`
	Hop    int     `json:"hop"`
	Mels   int     `json:"mels"`
	FMin   float64 `json:"f_min"`
	FMax   float64 `json:"f_max"`
	Floor  float64 `json:"floor"`
}

// Validate reports the first field that breaks the FrontEndConfig contract at
// this sample rate, naming the field and the value found. A non-positive
// sample rate is itself an error, since it leaves the band undefined.
func (c FrontEndConfig) Validate(sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("audio: front end sample rate must be positive, got %d Hz", sampleRate)
	}
	if err := c.validateFraming(); err != nil {
		return err
	}
	if c.Mels < 1 {
		return fmt.Errorf("audio: front end needs at least one mel filter, got %d", c.Mels)
	}
	nyquist := float64(sampleRate) / 2
	// Written as a negated conjunction so a NaN edge is rejected too.
	if !(c.FMin >= 0 && c.FMin < c.FMax && c.FMax <= nyquist) {
		return fmt.Errorf("audio: front end band [%v, %v] Hz must rise inside [0, %v] Hz", c.FMin, c.FMax, nyquist)
	}
	if !(c.Floor > 0) {
		return fmt.Errorf("audio: front end floor %v must be positive", c.Floor)
	}
	return nil
}

// validateFraming checks the two fields STFT alone depends on, so STFT can
// reject a broken framing without being told the sample rate.
func (c FrontEndConfig) validateFraming() error {
	if c.Window < 16 || c.Window&(c.Window-1) != 0 {
		return fmt.Errorf("audio: front end window %d must be a power of two of at least 16", c.Window)
	}
	if c.Hop < 1 || c.Hop > c.Window {
		return fmt.Errorf("audio: front end hop %d outside 1..%d", c.Hop, c.Window)
	}
	return nil
}

// STFT returns the power spectrum |X|^2 per frame: frames = 1 +
// (len(x)-Window)/Hop (no padding; len(x) < Window is an error), each with
// Window/2+1 bins. The Hann window is w[n] = 0.5 - 0.5*cos(2*pi*n/Window), the
// periodic form, so w[0] is exactly 0 and the weights sum to Window/2. Only
// Window and Hop are read, so the sample rate never enters.
func STFT(x []float64, c FrontEndConfig) ([][]float64, error) {
	if err := c.validateFraming(); err != nil {
		return nil, err
	}
	if len(x) < c.Window {
		return nil, fmt.Errorf("audio: stft input has %d samples, fewer than the %d-sample window", len(x), c.Window)
	}
	w := hann(c.Window)
	frames := 1 + (len(x)-c.Window)/c.Hop
	bins := c.Window/2 + 1
	out := make([][]float64, frames)
	buf := make([]complex128, c.Window)
	for f := range out {
		start := f * c.Hop
		for n := range buf {
			buf[n] = complex(x[start+n]*w[n], 0)
		}
		fft(buf)
		row := make([]float64, bins)
		for k := range row {
			re, im := real(buf[k]), imag(buf[k])
			row[k] = re*re + im*im
		}
		out[f] = row
	}
	return out, nil
}

// MelFilterbank returns Mels rows of Window/2+1 weights: triangular filters
// with centres equally spaced on the mel scale between FMin and FMax; every row
// sums to a positive value. A filter so narrow that no FFT bin falls strictly
// inside it would break that guarantee, so it is reported as an error naming
// the filter rather than returned as a silent row of zeros.
func MelFilterbank(sampleRate int, c FrontEndConfig) ([][]float64, error) {
	if err := c.Validate(sampleRate); err != nil {
		return nil, err
	}
	// Mels+2 mel-spaced edges: filter m runs from edge m to edge m+2 and peaks
	// at edge m+1.
	edges := make([]float64, c.Mels+2)
	loMel, hiMel := hzToMel(c.FMin), hzToMel(c.FMax)
	for i := range edges {
		edges[i] = melToHz(loMel + (hiMel-loMel)*float64(i)/float64(c.Mels+1))
	}
	bins := c.Window/2 + 1
	freq := make([]float64, bins)
	for k := range freq {
		freq[k] = float64(k) * float64(sampleRate) / float64(c.Window)
	}

	bank := make([][]float64, c.Mels)
	for m := range bank {
		left, centre, right := edges[m], edges[m+1], edges[m+2]
		row := make([]float64, bins)
		sum := 0.0
		for k, f := range freq {
			var v float64
			switch {
			case f > left && f <= centre:
				v = (f - left) / (centre - left)
			case f > centre && f < right:
				v = (right - f) / (right - centre)
			}
			row[k] = v
			sum += v
		}
		if !(sum > 0) {
			return nil, fmt.Errorf("audio: mel filter %d spanning %.4f..%.4f Hz catches no FFT bin; use a longer window or fewer filters", m, left, right)
		}
		bank[m] = row
	}
	return bank, nil
}

// FrontEndReport declares the exact configuration a feature matrix came from,
// so a run report states the window, hop and filterbank version instead of
// leaving them implied.
type FrontEndReport struct {
	Version    string         `json:"version"`
	SampleRate int            `json:"sample_rate"`
	Config     FrontEndConfig `json:"config"`
	Frames     int            `json:"frames"`
}

// LogMel is log10(filterbank . power + Floor) per frame, [frames][Mels], with
// the report that declares the exact configuration. It is a pure function of
// x, sampleRate and c: two runs over the same input agree bit for bit.
func LogMel(x []float64, sampleRate int, c FrontEndConfig) ([][]float64, FrontEndReport, error) {
	bank, err := MelFilterbank(sampleRate, c)
	if err != nil {
		return nil, FrontEndReport{}, err
	}
	power, err := STFT(x, c)
	if err != nil {
		return nil, FrontEndReport{}, err
	}
	out := make([][]float64, len(power))
	for f, spec := range power {
		row := make([]float64, c.Mels)
		for m, filter := range bank {
			energy := 0.0
			for k, weight := range filter {
				energy += weight * spec[k]
			}
			row[m] = math.Log10(energy + c.Floor)
		}
		out[f] = row
	}
	report := FrontEndReport{
		Version:    FrontEndVersion,
		SampleRate: sampleRate,
		Config:     c,
		Frames:     len(out),
	}
	return out, report, nil
}

// hann builds the periodic Hann window of n samples,
// w[n] = 0.5 - 0.5*cos(2*pi*n/N).
func hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}

// fft replaces a with its discrete Fourier transform in place, by iterative
// radix-2 Cooley-Tukey. len(a) must be a power of two, which validateFraming
// guarantees for every caller here.
func fft(a []complex128) {
	n := len(a)
	// Bit-reversal permutation: j counts in reverse-carry order alongside i.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		angle := -2 * math.Pi / float64(size)
		half := size / 2
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				twiddle := cmplx.Rect(1, angle*float64(k))
				u := a[start+k]
				v := a[start+k+half] * twiddle
				a[start+k] = u + v
				a[start+k+half] = u - v
			}
		}
	}
}

// hzToMel maps a frequency onto the HTK mel scale, m = 2595*log10(1+f/700).
func hzToMel(f float64) float64 { return 2595 * math.Log10(1+f/700) }

// melToHz inverts hzToMel.
func melToHz(m float64) float64 { return 700 * (math.Pow(10, m/2595) - 1) }
