package usage

import "testing"

func TestCanceledRequestIsNotFailureOrMeasuredSpeed(t *testing.T) {
	r := row{Canceled: true, Status: 499, Err: "the agent canceled the request", Ms: 3000, Out: 2000, Speed: 150}
	if r.failed() {
		t.Fatal("cancellation counted as failure")
	}
	if got := requestSpeed(r); got.value != 0 || got.reason != "canceled" {
		t.Fatalf("cancelled request reported speed: %+v", got)
	}
	r.Canceled = false
	if !r.failed() {
		t.Fatal("host timeout with status 499 was hidden as cancellation")
	}
}
