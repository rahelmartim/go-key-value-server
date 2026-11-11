package kvserver

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		line  string
		want  command
		valid bool
	}{
		{"put,name,Ada\n", command{op: "put", key: "name", value: []byte("Ada")}, true},
		{"put,csv,a,b,c\n", command{op: "put", key: "csv", value: []byte("a,b,c")}, true},
		{"get,name\r\n", command{op: "get", key: "name"}, true},
		{"get\n", command{}, false},
		{"delete,name\n", command{}, false},
		{"\n", command{}, false},
	}
	for _, tt := range tests {
		got, valid := parseCommand(tt.line)
		if valid != tt.valid || got.op != tt.want.op || got.key != tt.want.key ||
			string(got.value) != string(tt.want.value) {
			t.Errorf("parseCommand(%q) = %+v, %v; want %+v, %v", tt.line, got, valid, tt.want, tt.valid)
		}
	}
}
