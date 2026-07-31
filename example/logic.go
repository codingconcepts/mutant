package example

func CanVote(age int, citizen bool) bool {
	return age >= 18 && citizen
}

func IsWeekendOrHoliday(weekend bool, holiday bool) bool {
	return weekend || holiday
}
