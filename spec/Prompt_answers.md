# Global
1. multi-user webapp
2. email/password login
3. Super admin, normal users / owner, property manager, read-only.
4. More granular per module
5. in v1 only for BE/API actions
6. One SQLite is enough, but the setup should assume later migration to PostgreSQL
7. Will be depeloyed on VPS, but there will be HTTPS, backups etc.

# Property Model
1. Property name, legal owner details, address, billing details, ICO / DIC / VAT, Nuki credentials, booking ICS URL, default check-in/check-out times, cleaning lady details
2. Each property is one rentable unit.
3. rom the owning user/business profile
4. yes

# Cleaning Log Module
1. one authID, that's fetched from Nuki API
2. For now there is only one, but in future there can be more.
3. Fully ignored
4. Data is reported only from Nuki
5. Nuki entry can't be missing. Cleaning always happen
6. imeediately
7. I can over ride it, e.g if I give her a bonus.
8. on-screen statistics
9. washing fee per cleaning visit.
10. include the filtering by propery/month/year

# Finance Module
1. date, direction(Incoming, Outgoing), amount, category
2. Always in EUR
3. Income always comes from booking, but we should be able to enter manually as well
4. create when I open the month
5. yes
6. only future months
7. yes
8. Yes, there are e.g returns on utility bill, we will differentiate based on category
9. cleaner salary / total monthly property income
10. no

# Invoice Module
1. Manually
2. Yes
3. Yes, slovak unvoicing numbers
4. per property and per year
5. Yes
6. these are non-VAT invoices for now.
7. Yes to both
8. yes
9. yes in DB plus file on disk.

# ICS / Occupancy Module
1. Design should allow airbnb and direct booking later
2. ICS should run hourly, link to ics should be configurable.
3. yes
4. yes
5. yes
6. Month calendar UI and list view
7. yes via token
8. Both expose JSON and also a direct integration, if it's simple to implement.

# Customer Message Templates
1. Messages are generic.
2. Name and other customer details won't be in the message, only the date which comes from ICS
3. English, Slovak, German, Ukranian, Hungarian.
4. Template should be editable in UI
5. Property specific
6. Property name, address, WiFI, parking, contact phone.
7. Check-in only for now.
8. Copy to clipboard is fine.

# Cross-Module Logic
1. Occupancy/stay
2. Continues draft, e.g if we check Nuki events daily, then we add as we go, but as a single entry.
3. Yes
4. yes

# Non-Functional
1. API design, DB schema proposal and UI screen breakdowns. Functional requrimenets
2. Output must be otpimized for a Single AI coding agent for a more production grade phased implementation.
3. Tests should be per module
4. No