package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ResolvesExactAndSharedChannels(t *testing.T) {
	// arrange: two projects, one shared channel (SYSREQ-011 shape).
	data := []byte(`
routes:
  - project: grp/app
    channel: "channel-one"
  - project: grp/lib
    channel: "channel-one"
`)

	// act
	table, err := Parse(data)
	require.NoError(t, err)

	// assert
	ch, ok := table.Channel("grp/app")
	assert.True(t, ok)
	assert.Equal(t, "channel-one", ch)

	ch, ok = table.Channel("grp/lib")
	assert.True(t, ok)
	assert.Equal(t, "channel-one", ch)

	_, ok = table.Channel("grp/missing")
	assert.False(t, ok, "unmapped paths must never resolve")

	_, ok = table.Channel("GRP/App")
	assert.False(t, ok, "resolution is case-sensitive")
}

func TestParse_RejectsBrokenTables(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"not yaml", "\t- :"},
		{"duplicate project", "routes:\n  - {project: a/b, channel: c1}\n  - {project: a/b, channel: c2}\n"},
		{"empty project", "routes:\n  - {project: \"\", channel: c1}\n"},
		{"empty channel", "routes:\n  - {project: a/b, channel: \"\"}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act & assert
			_, err := Parse([]byte(tc.data))
			require.Error(t, err)
		})
	}
}

func TestLoad_MissingFileNamesPath(t *testing.T) {
	// act
	_, err := Load("/nonexistent/routing.yaml")

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/nonexistent/routing.yaml")
}
