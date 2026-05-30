package sshclient

import "time"

func SlotWaitTimeout() time.Duration {
	return slotTimeOutHardWaitSlot
}
