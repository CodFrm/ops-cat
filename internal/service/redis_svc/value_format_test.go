package redis_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDisplayValue(t *testing.T) {
	t.Run("formats json object without changing storage semantics", func(t *testing.T) {
		got := FormatDisplayValue(`{"b":2,"a":1}`, RedisValueFormatJSON)

		assert.True(t, got.Valid)
		assert.Equal(t, RedisValueFormatJSON, got.Format)
		assert.Contains(t, got.Value, "\n")
		assert.Contains(t, got.Value, `"b": 2`)
		assert.Empty(t, got.Error)
	})

	t.Run("returns original value for invalid json", func(t *testing.T) {
		got := FormatDisplayValue(`{"broken"`, RedisValueFormatJSON)

		assert.False(t, got.Valid)
		assert.Equal(t, `{"broken"`, got.Value)
		assert.NotEmpty(t, got.Error)
	})

	t.Run("renders hex and base64 from raw bytes", func(t *testing.T) {
		assert.Equal(t, "6869", FormatDisplayValue("hi", RedisValueFormatHex).Value)
		assert.Equal(t, "aGk=", FormatDisplayValue("hi", RedisValueFormatBase64).Value)
	})

	t.Run("unknown format falls back to raw", func(t *testing.T) {
		got := FormatDisplayValue("plain", "bogus")

		assert.True(t, got.Valid)
		assert.Equal(t, RedisValueFormatRaw, got.Format)
		assert.Equal(t, "plain", got.Value)
	})
}

func TestEncodeValueForStorage(t *testing.T) {
	t.Run("json mode keeps exact editor text", func(t *testing.T) {
		input := "{\n  \"a\": 1\n}"

		got, err := EncodeValueForStorage(input, RedisValueFormatJSON)

		require.NoError(t, err)
		assert.Equal(t, input, got)
	})

	t.Run("raw mode keeps exact editor text", func(t *testing.T) {
		got, err := EncodeValueForStorage("hello world", RedisValueFormatRaw)

		require.NoError(t, err)
		assert.Equal(t, "hello world", got)
	})

	t.Run("hex and base64 modes decode explicit encoded input", func(t *testing.T) {
		hexValue, err := EncodeValueForStorage("6869", RedisValueFormatHex)
		require.NoError(t, err)
		assert.Equal(t, "hi", hexValue)

		base64Value, err := EncodeValueForStorage("aGk=", RedisValueFormatBase64)
		require.NoError(t, err)
		assert.Equal(t, "hi", base64Value)
	})

	t.Run("invalid encoded values return an error", func(t *testing.T) {
		_, err := EncodeValueForStorage("not-hex", RedisValueFormatHex)
		assert.Error(t, err)

		_, err = EncodeValueForStorage("not base64", RedisValueFormatBase64)
		assert.Error(t, err)
	})
}
