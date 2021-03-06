package node

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSlowLimiter_CanPass(t *testing.T) {
	type fields struct {
		slowCounter int64
		limiterOn   int32
		slow100s    map[string]int64
		slow50s     map[string]int64
		slow10s     map[string]int64
		lastSlowTs  int64
	}
	type args struct {
		cmd    string
		prefix string
	}
	slow100s := make(map[string]int64)
	slow50s := make(map[string]int64)
	slow10s := make(map[string]int64)
	slow100sTestTable := make(map[string]int64)
	slow100sTestTable["set test_table"] = 20
	slow50sTestTable := make(map[string]int64)
	slow50sTestTable["set test_table"] = 20
	slow10sTestTable := make(map[string]int64)
	slow10sTestTable["set test_table"] = 20
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// real no slow
		{"canpass_noslow1", fields{0, 1, slow100s, slow50s, slow10s, 0}, args{"set", "test_table"}, true},
		// no recorded table
		{"canpass_noslow_record", fields{maxSlowThreshold, 1, slow100s, slow50s, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, true},
		// last slow is long ago
		{"canpass_slow_last_long_ago", fields{maxSlowThreshold, 1, slow100sTestTable, slow50sTestTable, slow10sTestTable, time.Now().Add(-1 * time.Hour).UnixNano()}, args{"set", "test_table"}, true},
		// mid slow should only refuce 100ms write
		{"canpass_below100ms_in_small_slow", fields{smallSlowThreshold, 1, slow100s, slow50sTestTable, slow10sTestTable, time.Now().UnixNano()}, args{"set", "test_table"}, true},
		{"cannotpass_100ms_in_small_slow", fields{smallSlowThreshold, 1, slow100sTestTable, slow50s, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, false},
		{"canpass_below50ms_in_mid_slow", fields{midSlowThreshold, 1, slow100s, slow50s, slow10sTestTable, time.Now().UnixNano()}, args{"set", "test_table"}, true},
		{"cannotpass_50ms_in_mid_slow", fields{midSlowThreshold, 1, slow100s, slow50sTestTable, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, false},
		{"cannotpass_100ms_in_mid_slow", fields{midSlowThreshold, 1, slow100sTestTable, slow50s, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, false},
		{"canpass_below10ms_in_heavy_slow", fields{heavySlowThreshold, 1, slow100s, slow50s, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, true},
		{"cannotpass_10ms_in_heavy_slow", fields{heavySlowThreshold, 1, slow100s, slow50s, slow10sTestTable, time.Now().UnixNano()}, args{"set", "test_table"}, false},
		{"cannotpass_50ms_in_heavy_slow", fields{heavySlowThreshold, 1, slow100s, slow50sTestTable, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, false},
		{"cannotpass_100ms_in_heavy_slow", fields{heavySlowThreshold, 1, slow100sTestTable, slow50s, slow10s, time.Now().UnixNano()}, args{"set", "test_table"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := &SlowLimiter{
				slowCounter: tt.fields.slowCounter,
				limiterOn:   tt.fields.limiterOn,
				slow100s:    tt.fields.slow100s,
				slow50s:     tt.fields.slow50s,
				slow10s:     tt.fields.slow10s,
				lastSlowTs:  tt.fields.lastSlowTs,
			}
			if got := sl.CanPass(time.Now().UnixNano(), tt.args.cmd, tt.args.prefix); got != tt.want {
				t.Errorf("SlowLimiter.CanPass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlowLimiter_SlowToNoSlow(t *testing.T) {
	sl := NewSlowLimiter()
	sl.Start()
	defer sl.Stop()
	cnt := 0
	atomic.StoreInt64(&sl.slowCounter, midSlowThreshold)
	oldTs := time.Now().UnixNano()
	atomic.StoreInt64(&sl.lastSlowTs, oldTs)
	sl.RecordSlowCmd("test", "test_table", SlowRefuseCost)
	sl.RecordSlowCmd("test", "test_table", SlowRefuseCost)
	sl.RecordSlowCmd("test", "test_table", SlowRefuseCost)
	assert.True(t, !sl.CanPass(time.Now().UnixNano(), "test", "test_table"))
	// use old ts to check pass to make sure we are passed by the cleared slow record
	for {
		cnt++
		if sl.CanPass(time.Now().UnixNano(), "test", "test_table") && sl.CanPass(oldTs, "test", "test_table") {
			break
		}
		time.Sleep(time.Second)
	}
	t.Logf("slow to noslow cnt : %v", cnt)
	assert.True(t, cnt >= smallSlowThreshold)
	assert.True(t, cnt < heavySlowThreshold)
}

func TestSlowLimiter_NoSlowToSlow(t *testing.T) {
	sl := NewSlowLimiter()
	sl.Start()
	defer sl.Stop()
	cnt := 0
	for {
		sl.RecordSlowCmd("test", "test_table", SlowRefuseCost)
		sl.MaybeAddSlow(time.Now().UnixNano(), SlowRefuseCost)
		cnt++
		if !sl.CanPass(time.Now().UnixNano(), "test", "test_table") {
			break
		}
	}
	t.Logf("noslow to slow cnt : %v", cnt)
	assert.True(t, cnt >= smallSlowThreshold)
	assert.True(t, cnt < heavySlowThreshold)
}
