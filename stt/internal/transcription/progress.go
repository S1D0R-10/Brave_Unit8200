package transcription

import (
	"regexp"
	"strconv"
	"time"
)

var whisperProgressPattern = regexp.MustCompile(`progress\s*=\s*([0-9]{1,3})%`)

func ParseWhisperProgress(line string) (int, bool) {
	match := whisperProgressPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value < 0 || value > 100 {
		return 0, false
	}
	return value, true
}

type Estimate struct {
	ETA            *time.Duration
	RealtimeFactor *float64
}

type ProgressEstimator struct {
	minElapsed  time.Duration
	minPercent  int
	startedAt   time.Time
	smoothedRTF float64
}

func RealtimeFactor(durationMS int64, wall time.Duration) float64 {
	if durationMS <= 0 || wall <= 0 {
		return 0
	}
	return (float64(durationMS) / 1000) / wall.Seconds()
}

func NewProgressEstimator(minElapsed time.Duration, minPercent int) *ProgressEstimator {
	return &ProgressEstimator{minElapsed: minElapsed, minPercent: minPercent}
}

func (e *ProgressEstimator) Observe(now time.Time, percent int, duration time.Duration) Estimate {
	if e.startedAt.IsZero() {
		e.startedAt = now
		return Estimate{}
	}
	elapsed := now.Sub(e.startedAt)
	if elapsed < e.minElapsed || percent < e.minPercent || percent <= 0 || percent >= 100 {
		return Estimate{}
	}
	processed := duration.Seconds() * float64(percent) / 100
	instantRTF := processed / elapsed.Seconds()
	if instantRTF <= 0 {
		return Estimate{}
	}
	if e.smoothedRTF == 0 {
		e.smoothedRTF = instantRTF
	} else {
		e.smoothedRTF = .2*instantRTF + .8*e.smoothedRTF
	}
	remaining := duration.Seconds() * float64(100-percent) / 100 / e.smoothedRTF
	eta := time.Duration(remaining * float64(time.Second))
	rtf := e.smoothedRTF
	return Estimate{ETA: &eta, RealtimeFactor: &rtf}
}
