package core

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultCommunityMood(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	mood, err := DefaultCommunityMood("guild-001", now)
	if err != nil {
		t.Fatalf("DefaultCommunityMood() error = %v", err)
	}

	if mood.Category != CategoryPlayful || mood.Intensity != 2 {
		t.Fatalf("default mood = (%s, %d), want (playful, 2)", mood.Category, mood.Intensity)
	}
}

func TestNewCommunityMoodValidation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		mood    CommunityMood
		wantErr error
	}{
		{
			name:    "empty guild ID",
			mood:    CommunityMood{Category: CategoryPlayful, Intensity: 2, CreatedAt: now, UpdatedAt: now},
			wantErr: ErrInvalidGuildID,
		},
		{
			name:    "invalid category",
			mood:    CommunityMood{GuildID: "g1", Category: "ecstatic", Intensity: 2, CreatedAt: now, UpdatedAt: now},
			wantErr: ErrInvalidMoodCategory,
		},
		{
			name:    "intensity below 1",
			mood:    CommunityMood{GuildID: "g1", Category: CategoryPlayful, Intensity: 0, CreatedAt: now, UpdatedAt: now},
			wantErr: ErrInvalidIntensity,
		},
		{
			name:    "intensity above 5",
			mood:    CommunityMood{GuildID: "g1", Category: CategoryPlayful, Intensity: 6, CreatedAt: now, UpdatedAt: now},
			wantErr: ErrInvalidIntensity,
		},
		{
			name:    "zero timestamp",
			mood:    CommunityMood{GuildID: "g1", Category: CategoryPlayful, Intensity: 2},
			wantErr: ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewCommunityMood(tt.mood); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyDecay(t *testing.T) {
	base := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		initialCat    Category
		initialInt    int
		elapsed       time.Duration
		wantCategory  Category
		wantIntensity int
	}{
		{
			name:          "under 6 hours does not decay",
			initialCat:    CategoryCheerful,
			initialInt:    4,
			elapsed:       5*time.Hour + 59*time.Minute,
			wantCategory:  CategoryCheerful,
			wantIntensity: 4,
		},
		{
			name:          "exactly 6 hours reduces intensity by 1",
			initialCat:    CategoryCheerful,
			initialInt:    4,
			elapsed:       6 * time.Hour,
			wantCategory:  CategoryCheerful,
			wantIntensity: 3,
		},
		{
			name:          "18 hours reduces intensity by 3",
			initialCat:    CategoryAnnoyed,
			initialInt:    4,
			elapsed:       18 * time.Hour,
			wantCategory:  CategoryAnnoyed,
			wantIntensity: 1,
		},
		{
			name:          "falling below 1 resets to default playful intensity 2",
			initialCat:    CategoryAnnoyed,
			initialInt:    2,
			elapsed:       12 * time.Hour,
			wantCategory:  CategoryPlayful,
			wantIntensity: 2,
		},
		{
			name:          "long inactivity resets to default playful intensity 2",
			initialCat:    CategoryTired,
			initialInt:    5,
			elapsed:       36 * time.Hour,
			wantCategory:  CategoryPlayful,
			wantIntensity: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mood := CommunityMood{
				GuildID:   "guild-1",
				Category:  tt.initialCat,
				Intensity: tt.initialInt,
				CreatedAt: base,
				UpdatedAt: base,
			}

			decayed := ApplyDecay(mood, base.Add(tt.elapsed))
			if decayed.Category != tt.wantCategory || decayed.Intensity != tt.wantIntensity {
				t.Errorf(
					"ApplyDecay() = (%s, %d), want (%s, %d)",
					decayed.Category, decayed.Intensity,
					tt.wantCategory, tt.wantIntensity,
				)
			}
		})
	}
}

func TestDetectSignal(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Signal
	}{
		{name: "Thai greeting", content: "<@bot> สวัสดีตอนเช้าครับ", want: SignalGreeting},
		{name: "English compliment", content: "<@bot> thanks you are awesome", want: SignalGreeting},
		{name: "Thai playful laughter", content: "<@bot> 555 ตลกมากเลย", want: SignalPlayful},
		{name: "English playful joke", content: "<@bot> haha nice tease", want: SignalPlayful},
		{name: "English sarcasm", content: "<@bot> yeah right, sure buddy", want: SignalSarcastic},
		{name: "Thai sarcasm", content: "<@bot> เก่งจังเลยนะวันนี้", want: SignalSarcastic},
		{name: "English hostility", content: "<@bot> shut up you are annoying", want: SignalHostile},
		{name: "Thai hostility", content: "<@bot> หุบปากไปเลย", want: SignalHostile},
		{name: "Thai tiredness", content: "<@bot> วันนี้ง่วงมาก หมดแรงแล้ว", want: SignalTired},
		{name: "English tiredness", content: "<@bot> I am so tired today", want: SignalTired},
		{name: "Standalone 555", content: "<@bot> 555", want: SignalPlayful},
		{name: "Standalone 5555", content: "<@bot> 5555!", want: SignalPlayful},
		{name: "Neutral question causes no signal", content: "<@bot> what time does the game start?", want: SignalNone},
		{name: "Ambiguous short mention causes no signal", content: "<@bot> hey", want: SignalNone},
		// False positive regressions:
		{name: "URL path containing /s does not trigger sarcasm", content: "check https://example.com/s/article please", want: SignalNone},
		{name: "Removed /s cue does not trigger sarcasm", content: "nice job /s", want: SignalNone},
		{name: "URL containing tired does not trigger tired", content: "read https://example.com/tired-guide", want: SignalNone},
		{name: "retired does not match tired", content: "my dad retired last year", want: SignalNone},
		{name: "15555 does not match 555", content: "order number 15555 arrived", want: SignalNone},
		{name: "5550 does not match 555", content: "port 5550 is open", want: SignalNone},
		{name: "phone with 555 does not match 555", content: "call 085-555-1234", want: SignalNone},
		{name: "lollipop does not match lol", content: "I bought a lollipop", want: SignalNone},
		{name: "thanksgiving does not match thanks", content: "happy thanksgiving dinner", want: SignalNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectSignal(tt.content)
			if got != tt.want {
				t.Errorf("DetectSignal(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

func TestApplySignalToCommunityAndTemporary(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	base, _ := DefaultCommunityMood("guild-1", now)

	// Neutral causes no mutation.
	unchanged, changed := ApplySignalToCommunity(base, SignalNone, now.Add(time.Minute))
	if changed || unchanged != base {
		t.Fatalf("SignalNone should not change community mood")
	}

	// Playful reinforces playful up to 5.
	m, changed := ApplySignalToCommunity(base, SignalPlayful, now.Add(time.Minute))
	if !changed || m.Category != CategoryPlayful || m.Intensity != 3 {
		t.Fatalf("expected (playful, 3), got (%s, %d)", m.Category, m.Intensity)
	}

	// Hostile softens positive mood first before flipping to annoyed.
	m, changed = ApplySignalToCommunity(m, SignalHostile, now.Add(2*time.Minute))
	if !changed || m.Category != CategoryPlayful || m.Intensity != 2 {
		t.Fatalf("expected (playful, 2), got (%s, %d)", m.Category, m.Intensity)
	}

	// Repeated hostile transitions down to annoyed and caps at 5.
	m.Intensity = 1
	m, _ = ApplySignalToCommunity(m, SignalHostile, now.Add(3*time.Minute))
	if m.Category != CategoryAnnoyed || m.Intensity != 1 {
		t.Fatalf("expected (annoyed, 1), got (%s, %d)", m.Category, m.Intensity)
	}

	// Greeting softens annoyed back to neutral.
	m, _ = ApplySignalToCommunity(m, SignalGreeting, now.Add(4*time.Minute))
	if m.Category != CategoryNeutral || m.Intensity != 1 {
		t.Fatalf("expected (neutral, 1), got (%s, %d)", m.Category, m.Intensity)
	}

	// Temporary mood sets 15-minute TTL and escalates on matching signal.
	temp, tempChanged := ApplySignalToTemporary("guild-1", "ch-1", nil, SignalSarcastic, now)
	if !tempChanged || temp == nil {
		t.Fatal("expected temporary mood to be created")
	}
	if temp.Category != CategorySarcastic || temp.Intensity != 3 {
		t.Errorf("temp = (%s, %d), want (sarcastic, 3)", temp.Category, temp.Intensity)
	}
	if !temp.ExpiresAt.Equal(now.Add(TemporaryMoodTTL)) {
		t.Errorf("ExpiresAt = %v, want %v", temp.ExpiresAt, now.Add(TemporaryMoodTTL))
	}

	temp2, _ := ApplySignalToTemporary("guild-1", "ch-1", temp, SignalSarcastic, now.Add(time.Minute))
	if temp2.Intensity != 4 {
		t.Errorf("escalated temp intensity = %d, want 4", temp2.Intensity)
	}

	// CombineMoods ignores expired temporary mood.
	combinedActive := CombineMoods(base, temp2, now.Add(5*time.Minute))
	if combinedActive.EffectiveCategory != CategorySarcastic || combinedActive.EffectiveIntensity != 4 {
		t.Errorf("unexpected active combined mood: %+v", combinedActive)
	}

	combinedExpired := CombineMoods(base, temp2, temp2.ExpiresAt)
	if combinedExpired.EffectiveCategory != base.Category || combinedExpired.Temporary != nil {
		t.Errorf("expired temporary mood should not apply: %+v", combinedExpired)
	}
}
