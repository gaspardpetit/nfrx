package worker

import (
	"testing"

"github.com/gaspardpetit/nfrx/core/reconnect"
)

func TestReconnectDelay(t *testing.T) {
	expected := []int{1, 1, 1, 5, 5, 5, 15, 15, 15, 30, 30}
	for i, exp := range expected {
		d := reconnect.Delay(i)
		if int(d.Seconds()) != exp {
			t.Errorf("attempt %d: expected %d got %v", i, exp, d)
		}
	}
}
