I have put 2 files for you to look at and suggest a business logic for a coding AI agent. You must act as a business analyst that got a new request from the business to implement this feature. Point out and ask any clarifications that you need for making this bullet proof.

At the moment in the Finance module, I can upload booking payouts that will populate Occupancy and Finances and some analytics data. there are useful data in there.

See the example file: September_PayoutInfo.csv, we have full support for processing this file.

However I found that there are also Booking statements that have a lot more interesting data.

I want you to come up with these things:
* We must support uploading data from both data file types.
* When uploaded we must identify, if we already don't have data for these from one of the files before.
* If we do, we just enhance the existing data with new data that were not present in one of the files.
* Example: Statement file contains column "Booked on" which can be used to calculate lead time of a booking.
* Example: Statement file contains column "Status" so we can identify cancelation.
* Example: Statement file contains column "Commision" I would like to see in Analytics module average commision rate and a chart of comission per stay same as we have with "Net stay" chart.
* Example: Statement file contains column "Persons", I want to see the break down, how many stays are for how many people and also ADR per perons stay, e.g what's ADR of 1 person stays, 2 person stays etc.

make sure you cover all the angles from the code perspective, analyst perspective and overall strategy of PMS.