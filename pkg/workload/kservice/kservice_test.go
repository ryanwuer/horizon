package kservice

import (
	"reflect"
	"testing"
)

func TestBuildTrafficPercentArray(t *testing.T) {
	tests := []struct {
		stepTotal int
		want      []int
	}{
		{1, []int{100}},
		{2, []int{5, 95}},
		{3, []int{5, 47, 48}},
		{4, []int{5, 31, 31, 33}},
		{5, []int{5, 23, 23, 23, 26}},
		{6, []int{5, 19, 19, 19, 19, 19}},
	}

	for _, test := range tests {
		got, err := buildTrafficPercentArray(test.stepTotal)
		if err != nil {
			t.Errorf("buildTrafficPercentArray(%d) = %v", test.stepTotal, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("buildTrafficPercentArray(%d) = %v, want %v", test.stepTotal, got, test.want)
		}
	}
}
