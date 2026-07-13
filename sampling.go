package logging

import "go.uber.org/zap"

// Sampling controls Zap's per-second sampling to reduce CloudWatch costs.
//
// Zap groups log entries by (level, message). For each group, within every
// 1-second window, the first `Initial` entries are kept and after that 1 in
// every `Thereafter` entries is kept.
//
// To turn sampling off entirely, use Config.DisableSampling.
type Sampling struct {
	// Initial is the number of entries per (level, message) per second to
	// keep unconditionally before sampling kicks in.
	Initial int

	// Thereafter is the sampling ratio after `Initial` — 1 entry kept per
	// `Thereafter` matching entries. Set to 1 to keep everything (effectively
	// disables thinning once past Initial).
	Thereafter int
}

// DefaultSampling returns sensible production defaults:
// keep the first 100 of each (level, msg) per second, then 1 in every 100.
// At the volumes our services emit, this caps high-frequency logs (health
// checks, hot loops) at a predictable rate while still preserving full
// fidelity for anything that fires < 100x/sec.
func DefaultSampling() Sampling {
	return Sampling{Initial: 100, Thereafter: 100}
}

// toZap converts to Zap's sampling config. Returns nil if sampling is disabled.
func (s Sampling) toZap() *zap.SamplingConfig {
	if s.Initial <= 0 {
		return nil
	}
	thereafter := s.Thereafter
	if thereafter <= 0 {
		thereafter = 1
	}
	return &zap.SamplingConfig{
		Initial:    s.Initial,
		Thereafter: thereafter,
	}
}
