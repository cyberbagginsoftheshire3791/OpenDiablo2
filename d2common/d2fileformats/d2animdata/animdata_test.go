package d2animdata

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2datautils"
)

// The fixtures in this file are synthesized in code. No bytes here come from a
// Blizzard MPQ (Project Strigoi law: Blizzard content never enters the repo —
// see CLAUDE.md and docs/fixtures-manifest.md). The layout follows Load():
// 256 blocks, each a uint32 record count followed by records of
// 8-byte null-terminated name · uint32 frames · uint16 speed · 2 pad bytes ·
// 144 event bytes. Records are placed in the block selected by hashName, the
// way the real file is laid out.

type testRecord struct {
	name   string
	frames uint32
	speed  uint16
	events map[int]AnimationEvent
}

// synthesizeAnimData builds a valid AnimData.d2 byte stream from records,
// placing each record in its hash block.
func synthesizeAnimData(records []testRecord) []byte {
	blocks := make([][]testRecord, numBlocks)

	for _, r := range records {
		idx := hashName(r.name)
		blocks[idx] = append(blocks[idx], r)
	}

	sw := d2datautils.CreateStreamWriter()

	for _, blockRecords := range blocks {
		sw.PushUint32(uint32(len(blockRecords)))

		for _, r := range blockRecords {
			nameBytes := make([]byte, byteCountName)
			copy(nameBytes, r.name)
			sw.PushBytes(nameBytes...)

			sw.PushUint32(r.frames)
			sw.PushUint16(r.speed)

			for i := 0; i < byteCountSpeedPadding; i++ {
				sw.PushBytes(0)
			}

			for event := 0; event < numEvents; event++ {
				sw.PushBytes(byte(r.events[event]))
			}
		}
	}

	return sw.GetBytes()
}

// goodRecords is a small but structurally complete data set: several names,
// repeated names (the real file has many records per name), non-zero events,
// and a speed of exactly speedDivisor so FPS() can be checked against
// speedBaseFPS.
func goodRecords() []testRecord {
	return []testRecord{
		{name: "STRNU1A", frames: 8, speed: 256, events: map[int]AnimationEvent{3: AnimationEventSound}},
		{name: "STRNU1A", frames: 12, speed: 192, events: map[int]AnimationEvent{}},
		{name: "STRWL1A", frames: 16, speed: 320, events: map[int]AnimationEvent{7: AnimationEventAttack, 9: AnimationEventMissile}},
		{name: "VLGDD1A", frames: 20, speed: 128, events: map[int]AnimationEvent{19: AnimationEventSkill}},
		{name: "VLGGH1A", frames: 6, speed: 256, events: map[int]AnimationEvent{}},
	}
}

func TestLoad(t *testing.T) {
	data := synthesizeAnimData(goodRecords())

	ad, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}

	if ad.GetRecordsCount() != 4 {
		t.Fatalf("expected 4 distinct names, got %d", ad.GetRecordsCount())
	}

	records := ad.GetRecords("STRNU1A")
	if len(records) != 2 {
		t.Fatalf("expected 2 records for STRNU1A, got %d", len(records))
	}

	if records[0].FramesPerDirection() != 8 || records[0].Speed() != 256 {
		t.Errorf("first STRNU1A record mismatch: frames %d speed %d",
			records[0].FramesPerDirection(), records[0].Speed())
	}

	if records[0].Event(3) != AnimationEventSound {
		t.Error("event at frame 3 of the first STRNU1A record should be a sound event")
	}

	if records[0].Event(4) != AnimationEventNone {
		t.Error("frames without an event byte must read as AnimationEventNone")
	}

	// GetRecord returns the last record with the name
	if ad.GetRecord("STRNU1A").Speed() != 192 {
		t.Error("GetRecord should return the last record for a name")
	}

	if ad.GetRecord("STRWL1A").FPS() != float64(speedBaseFPS)*320/float64(speedDivisor) {
		t.Error("FPS mismatch for STRWL1A")
	}

	if ad.GetRecord("NOPE") != nil {
		t.Error("unknown names must return nil")
	}
}

func TestLoad_BlockAtCapacity(t *testing.T) {
	// maxRecordsPerBlock records that all hash to the same block must load;
	// one more must be rejected by the count check.
	records := make([]testRecord, 0, maxRecordsPerBlock)

	for i := 0; i < maxRecordsPerBlock; i++ {
		records = append(records, testRecord{name: "CAPBLK", frames: uint32(i), speed: 1, events: map[int]AnimationEvent{}})
	}

	if _, err := Load(synthesizeAnimData(records)); err != nil {
		t.Fatalf("a block with exactly %d records should load: %v", maxRecordsPerBlock, err)
	}

	records = append(records, testRecord{name: "CAPBLK", frames: 99, speed: 1, events: map[int]AnimationEvent{}})

	if _, err := Load(synthesizeAnimData(records)); err == nil {
		t.Fatalf("a block with %d records should be rejected", maxRecordsPerBlock+1)
	}
}

func TestLoad_BadData(t *testing.T) {
	good := synthesizeAnimData(goodRecords())

	truncated := make([]byte, len(good)-1)
	copy(truncated, good)

	trailing := make([]byte, len(good)+1)
	copy(trailing, good)

	// first block: one record whose 8-byte name has no null terminator
	noTerminator := make([]byte, 0)
	noTerminator = append(noTerminator, 1, 0, 0, 0)
	noTerminator = append(noTerminator, []byte("ABCDEFGH")...)
	noTerminator = append(noTerminator, make([]byte, 4+2+2+numEvents)...)

	for i := 1; i < numBlocks; i++ {
		noTerminator = append(noTerminator, 0, 0, 0, 0)
	}

	// first block claims more records than the format allows
	overCount := make([]byte, 0)
	overCount = append(overCount, maxRecordsPerBlock+1, 0, 0, 0)

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"truncated by one byte", truncated},
		{"one trailing byte", trailing},
		{"name without null terminator", noTerminator},
		{"record count over the block maximum", overCount},
	}

	for _, c := range cases {
		if _, err := Load(c.data); err == nil {
			t.Errorf("%s: bad data should not parse", c.name)
		}
	}
}

func TestAnimationData_GetRecordNames(t *testing.T) {
	animdata := &AnimationData{
		hashTable: hashTable{},
		blocks:    [256]*block{},
		entries: map[string][]*AnimationDataRecord{
			"a": {{}},
			"b": {{}},
			"c": {{}},
		},
	}

	names := animdata.GetRecordNames()
	if len(names) != 3 {
		t.Error("record name count mismatch")
	}
}

func TestAnimationData_GetRecords(t *testing.T) {
	animdata := &AnimationData{
		hashTable: hashTable{},
		blocks:    [256]*block{},
		entries: map[string][]*AnimationDataRecord{
			"a": {
				{name: "a", speed: 1, framesPerDirection: 1},
				{name: "a", speed: 2, framesPerDirection: 2},
				{name: "a", speed: 3, framesPerDirection: 3},
			},
		},
	}

	if len(animdata.GetRecords("a")) != 3 {
		t.Error("record count is incorrect")
	}

	if len(animdata.GetRecords("b")) > 0 {
		t.Error("retrieved records for unknown record name")
	}
}

func TestAnimationData_GetRecord(t *testing.T) {
	animdata := &AnimationData{
		hashTable: hashTable{},
		blocks:    [256]*block{},
		entries: map[string][]*AnimationDataRecord{
			"a": {
				{name: "a", speed: 1, framesPerDirection: 1},
				{name: "a", speed: 2, framesPerDirection: 2},
				{name: "a", speed: 3, framesPerDirection: 3},
			},
		},
	}

	record := animdata.GetRecord("a")
	if record.speed != 3 {
		t.Error("record returned is incorrect")
	}
}

func TestAnimationDataRecord_FPS(t *testing.T) {
	record := &AnimationDataRecord{}

	var fps float64

	record.speed = 256
	fps = record.FPS()

	if fps != float64(speedBaseFPS) {
		t.Error("incorrect fps")
	}

	record.speed = 512
	fps = record.FPS()

	if fps != float64(speedBaseFPS)*2 {
		t.Error("incorrect fps")
	}

	record.speed = 128
	fps = record.FPS()

	if fps != float64(speedBaseFPS)/2 {
		t.Error("incorrect fps")
	}
}

func TestAnimationData_Marshal(t *testing.T) {
	data := synthesizeAnimData(goodRecords())

	ad, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}

	newData := ad.Marshal()

	newAd, err := Load(newData)
	if err != nil {
		t.Fatal(err)
	}

	if len(newAd.entries) != len(ad.entries) {
		t.Fatalf("unexpected length of keys in first and second dict: %d, %d", len(ad.entries), len(newAd.entries))
	}

	for key := range newAd.entries {
		if len(newAd.entries[key]) != len(ad.entries[key]) {
			t.Fatalf("record count for %s changed across marshal: %d != %d",
				key, len(ad.entries[key]), len(newAd.entries[key]))
		}

		for n, i := range newAd.entries[key] {
			if i.speed != ad.entries[key][n].speed {
				t.Fatal("unexpected record set")
			}

			if i.framesPerDirection != ad.entries[key][n].framesPerDirection {
				t.Fatal("frames per direction changed across marshal")
			}

			for event, kind := range ad.entries[key][n].events {
				if i.Event(event) != kind {
					t.Fatalf("event %d of %s changed across marshal", event, key)
				}
			}
		}
	}
}

func TestAnimationData_DeleteRecord(t *testing.T) {
	ad := &AnimationData{
		entries: map[string][]*AnimationDataRecord{
			"a": {
				{name: "a", speed: 1, framesPerDirection: 1},
				{name: "a", speed: 2, framesPerDirection: 2},
				{name: "a", speed: 3, framesPerDirection: 3},
			},
		},
	}

	err := ad.DeleteRecord("a", 1)

	if err != nil {
		t.Error(err)
	}

	if len(ad.entries["a"]) != 2 {
		t.Fatal("Delete record error")
	}

	if ad.entries["a"][1].speed != 3 {
		t.Fatal("Invalid index deleted")
	}
}

func TestAnimationData_PushRecord(t *testing.T) {
	ad := &AnimationData{
		entries: map[string][]*AnimationDataRecord{
			"a": {
				{name: "a", speed: 1, framesPerDirection: 1},
				{name: "a", speed: 2, framesPerDirection: 2},
			},
		},
	}

	ad.PushRecord("a")

	if len(ad.entries["a"]) != 3 {
		t.Fatal("No record was pushed")
	}

	if ad.entries["a"][2].name != "a" {
		t.Fatal("unexpected name of new record was set")
	}
}

func TestAnimationData_AddEntry(t *testing.T) {
	ad := &AnimationData{
		entries: make(map[string][]*AnimationDataRecord),
	}

	err := ad.AddEntry("a")
	if err != nil {
		t.Error(err)
	}

	if _, found := ad.entries["a"]; !found {
		t.Fatal("entry wasn't added")
	}
}

func TestAnimationData_DeleteEntry(t *testing.T) {
	ad := &AnimationData{
		entries: map[string][]*AnimationDataRecord{
			"a": {{}, {}},
		},
	}

	err := ad.DeleteEntry("a")
	if err != nil {
		t.Error(err)
	}

	if _, found := ad.entries["a"]; found {
		t.Fatal("Entry wasn't deleted")
	}
}
