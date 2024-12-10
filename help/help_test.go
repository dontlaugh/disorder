package help

import "testing"

func TestJoinIf(t *testing.T) {

	tests := []struct {
		name string
		args []*string
		want string
	}{
		{
			name: "Nil yields a default",
			args: []*string{nil, nil, nil},
			want: "ALL_NIL",
		},
		{
			name: "Non nils join",
			args: []*string{nil, nil, strPtr("John"), nil, strPtr("Smith"), nil},
			want: "John Smith",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinIf(tt.args...); got != tt.want {
				t.Errorf("JoinIf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
