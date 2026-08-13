package store

import "time"

func dateNights(start, end string) ([]string, error) {
	checkIn, checkOut, err := parseNamedStayRange(start, end)
	if err != nil {
		return nil, err
	}
	nights := make([]string, 0, int(checkOut.Sub(checkIn)/(24*time.Hour)))
	for day := checkIn; day.Before(checkOut); day = day.AddDate(0, 0, 1) {
		nights = append(nights, day.Format("2006-01-02"))
	}
	return nights, nil
}
