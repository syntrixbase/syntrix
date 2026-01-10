package encoding

import (
	"bytes"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncode_Version(t *testing.T) {
	key, err := Encode(nil, "doc1")
	require.NoError(t, err)
	assert.Equal(t, Version, key[0], "first byte should be version")
}

func TestEncode_DocID(t *testing.T) {
	// Empty fields, just doc ID
	key, err := Encode(nil, "hello")
	require.NoError(t, err)

	// [ver:1][id_len:2][id:5]
	assert.Len(t, key, 1+2+5)
	assert.Equal(t, byte(0), key[1]) // high byte of length
	assert.Equal(t, byte(5), key[2]) // low byte of length
	assert.Equal(t, "hello", string(key[3:]))
}

func TestEncode_TypeTags(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantTag byte
	}{
		{"null", nil, TypeNull},
		{"bool", true, TypeBool},
		{"number", 42.0, TypeNumber},
		{"string", "test", TypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := Encode([]Field{{Value: tt.value, Direction: Asc}}, "id")
			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, key[1], "type tag mismatch")
		})
	}
}

func TestEncode_TypeOrdering(t *testing.T) {
	// Type ordering should be: null < bool < number < string
	nullKey, _ := Encode([]Field{{Value: nil, Direction: Asc}}, "a")
	boolKey, _ := Encode([]Field{{Value: false, Direction: Asc}}, "a")
	numKey, _ := Encode([]Field{{Value: 0.0, Direction: Asc}}, "a")
	strKey, _ := Encode([]Field{{Value: "", Direction: Asc}}, "a")

	assert.Equal(t, -1, Compare(nullKey, boolKey), "null < bool")
	assert.Equal(t, -1, Compare(boolKey, numKey), "bool < number")
	assert.Equal(t, -1, Compare(numKey, strKey), "number < string")
}

func TestEncode_BoolOrdering(t *testing.T) {
	falseKey, _ := Encode([]Field{{Value: false, Direction: Asc}}, "a")
	trueKey, _ := Encode([]Field{{Value: true, Direction: Asc}}, "a")

	assert.Equal(t, -1, Compare(falseKey, trueKey), "false < true in asc")
}

func TestEncode_NumberOrdering(t *testing.T) {
	tests := []struct {
		a, b float64
		want int
	}{
		{-100, -50, -1},
		{-1, 0, -1},
		{0, 1, -1},
		{1, 100, -1},
		{math.Inf(-1), -1000, -1},
		{1000, math.Inf(1), -1},
		{-0.5, 0.5, -1},
	}

	for _, tt := range tests {
		keyA, _ := Encode([]Field{{Value: tt.a, Direction: Asc}}, "x")
		keyB, _ := Encode([]Field{{Value: tt.b, Direction: Asc}}, "x")
		assert.Equal(t, tt.want, Compare(keyA, keyB), "%v < %v", tt.a, tt.b)
	}
}

func TestEncode_StringOrdering(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "a", -1},
		{"a", "b", -1},
		{"a", "aa", -1},
		{"ab", "b", -1},
		{"abc", "abd", -1},
	}

	for _, tt := range tests {
		keyA, _ := Encode([]Field{{Value: tt.a, Direction: Asc}}, "x")
		keyB, _ := Encode([]Field{{Value: tt.b, Direction: Asc}}, "x")
		assert.Equal(t, tt.want, Compare(keyA, keyB), "%q < %q", tt.a, tt.b)
	}
}

func TestEncode_DescInvertsOrder(t *testing.T) {
	// In desc mode, larger values should come first (smaller key)
	keySmall, _ := Encode([]Field{{Value: 10.0, Direction: Desc}}, "a")
	keyLarge, _ := Encode([]Field{{Value: 100.0, Direction: Desc}}, "a")

	// 100 should sort before 10 in desc (so 100's key < 10's key)
	assert.Equal(t, -1, Compare(keyLarge, keySmall), "100 should sort before 10 in desc")
}

func TestEncode_DescString(t *testing.T) {
	keyA, _ := Encode([]Field{{Value: "aaa", Direction: Desc}}, "x")
	keyB, _ := Encode([]Field{{Value: "bbb", Direction: Desc}}, "x")

	// "bbb" should sort before "aaa" in desc
	assert.Equal(t, -1, Compare(keyB, keyA), "bbb should sort before aaa in desc")
}

func TestEncode_DescStringLength(t *testing.T) {
	// This tests the fixed-width length encoding for desc
	// "abc" (len=3) vs "ab" (len=2) in desc should have "abc" first
	keyABC, _ := Encode([]Field{{Value: "abc", Direction: Desc}}, "x")
	keyAB, _ := Encode([]Field{{Value: "ab", Direction: Desc}}, "x")

	// In desc, "abc" > "ab" so "abc" should come first (smaller key)
	assert.Equal(t, -1, Compare(keyABC, keyAB), "abc should sort before ab in desc")
}

func TestEncode_MultipleFields(t *testing.T) {
	// name:asc, age:desc
	key1, _ := Encode([]Field{
		{Value: "alice", Direction: Asc},
		{Value: 30.0, Direction: Desc},
	}, "doc1")

	key2, _ := Encode([]Field{
		{Value: "alice", Direction: Asc},
		{Value: 25.0, Direction: Desc},
	}, "doc2")

	key3, _ := Encode([]Field{
		{Value: "bob", Direction: Asc},
		{Value: 20.0, Direction: Desc},
	}, "doc3")

	// alice < bob (asc), so key1, key2 < key3
	assert.Equal(t, -1, Compare(key1, key3), "alice < bob")
	assert.Equal(t, -1, Compare(key2, key3), "alice < bob")

	// For alice, age 30 > 25, so in desc 30 comes first
	assert.Equal(t, -1, Compare(key1, key2), "alice@30 < alice@25 in (name:asc, age:desc)")
}

func TestEncode_DocIDTiebreaker(t *testing.T) {
	// Same fields, different doc IDs
	key1, _ := Encode([]Field{{Value: "test", Direction: Asc}}, "aaa")
	key2, _ := Encode([]Field{{Value: "test", Direction: Asc}}, "bbb")

	assert.Equal(t, -1, Compare(key1, key2), "aaa < bbb as tiebreaker")
}

func TestEncode_Errors(t *testing.T) {
	t.Run("unsupported type", func(t *testing.T) {
		_, err := Encode([]Field{{Value: []int{1, 2, 3}, Direction: Asc}}, "id")
		assert.ErrorIs(t, err, ErrUnsupportedType)
	})

	t.Run("string too long", func(t *testing.T) {
		longStr := string(make([]byte, 65536))
		_, err := Encode([]Field{{Value: longStr, Direction: Asc}}, "id")
		assert.ErrorIs(t, err, ErrStringTooLong)
	})

	t.Run("doc ID too long", func(t *testing.T) {
		longID := string(make([]byte, 65536))
		_, err := Encode(nil, longID)
		assert.ErrorIs(t, err, ErrIDTooLong)
	})
}

func TestEncode_IntTypes(t *testing.T) {
	// int64 and int should work and produce same result as float64
	key1, _ := Encode([]Field{{Value: int64(42), Direction: Asc}}, "x")
	key2, _ := Encode([]Field{{Value: int(42), Direction: Asc}}, "x")
	key3, _ := Encode([]Field{{Value: 42.0, Direction: Asc}}, "x")

	assert.Equal(t, 0, Compare(key1, key2), "int64(42) == int(42)")
	assert.Equal(t, 0, Compare(key1, key3), "int64(42) == float64(42)")
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b []byte
		want int
	}{
		{[]byte{1, 2, 3}, []byte{1, 2, 3}, 0},
		{[]byte{1, 2, 3}, []byte{1, 2, 4}, -1},
		{[]byte{1, 2, 4}, []byte{1, 2, 3}, 1},
		{[]byte{1, 2}, []byte{1, 2, 3}, -1},
		{[]byte{1, 2, 3}, []byte{1, 2}, 1},
		{[]byte{}, []byte{}, 0},
		{[]byte{}, []byte{1}, -1},
	}

	for _, tt := range tests {
		got := Compare(tt.a, tt.b)
		assert.Equal(t, tt.want, got, "Compare(%v, %v)", tt.a, tt.b)
	}
}

func TestEncode_SortCorrectness(t *testing.T) {
	// Create a bunch of keys and verify they sort correctly
	type doc struct {
		name string
		age  float64
		id   string
	}

	docs := []doc{
		{"alice", 30, "d1"},
		{"alice", 25, "d2"},
		{"alice", 25, "d3"},
		{"bob", 20, "d4"},
		{"bob", 35, "d5"},
		{"charlie", 28, "d6"},
	}

	// Encode with name:asc, age:desc
	var keys [][]byte
	for _, d := range docs {
		key, err := Encode([]Field{
			{Value: d.name, Direction: Asc},
			{Value: d.age, Direction: Desc},
		}, d.id)
		require.NoError(t, err)
		keys = append(keys, key)
	}

	// Sort using bytes.Compare
	sorted := make([]int, len(keys))
	for i := range sorted {
		sorted[i] = i
	}
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(keys[sorted[i]], keys[sorted[j]]) < 0
	})

	// Expected order: alice@30, alice@25(d2), alice@25(d3), bob@35, bob@20, charlie@28
	expected := []string{"d1", "d2", "d3", "d5", "d4", "d6"}
	for i, idx := range sorted {
		assert.Equal(t, expected[i], docs[idx].id, "position %d", i)
	}
}
