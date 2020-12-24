package fastforward

import "testing"

func Test_parseAddr(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name          string
		args          args
		wantHost      string
		wantPreferTCP bool
		wantErr       bool
	}{
		{"empty protocol", args{addr: "8.8.8.8"}, "8.8.8.8:53", false, false},
		{"tcp protocol", args{addr: "tcp://8.8.8.8"}, "8.8.8.8:53", true, false},
		{"udp protocol", args{addr: "udp://8.8.8.8"}, "8.8.8.8:53", false, false},
		{"with port", args{addr: "8.8.8.8:5353"}, "8.8.8.8:5353", false, false},
		{"unsupported protocol", args{addr: "abc://8.8.8.8"}, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPreferTCP, err := parseAddr(tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotHost != tt.wantHost {
				t.Errorf("parseAddr() gotHost = %v, want %v", gotHost, tt.wantHost)
			}
			if gotPreferTCP != tt.wantPreferTCP {
				t.Errorf("parseAddr() gotPreferTCP = %v, want %v", gotPreferTCP, tt.wantPreferTCP)
			}
		})
	}
}
