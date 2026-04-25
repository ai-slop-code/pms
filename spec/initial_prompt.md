# PMS

I want to create something like a PMS software, e.g Property Management System, but my way!

I need you to act as a business analyst and create a specification based on my description for AI coding agent. You must consider everything, check for gaps and missing logic in my high level business specification. Before you give me specification you ask for any clarifications. Look at this from profesional angle you have to delivery a working specification which is easy and clear to follow by a developer AI agent.

Here is what I have in mind, I will split it in tech stack and actual business requirements that I have.

Tech stack:
There should be two parts, backend and frontend, backend must be written in Go. Frontend must be written in vue.js. Database storage should be SQLite.

Business idea:
My PMS needs to cover following core functionalities:
* It needs to act as logging system for my cleaning lady.
* It needs to keep a log book of all expensess and income for my property.
* It needs to generate PDF invoices for customers.
* It needs to sync iCalendar files, ics and display the property occupation.
* It needs to create Nuki access codes for the customers.
* It needs to generate mesages for customers from templates that can be copied to clip board.


Now I will go into more detail for each of the idea

## Global functionalities
These are some global fucntionalities that go beyond the module setup. We should be able to setup multiple properties within PMS. We must have user management and roles. Users can own multiple properties, they should see only the properties that they have permissions to, there is also admin role that can create users and access everything. Admins can create properties on behalf of the users, users can only create their own properties.

PMS must have a login, all API calls must be authentificated. We should also have logging implemented.

## Logging system for my cleaning lady - module
My cleaning lady enters the flat any time there is cleaning to be done. What I currently do is I manually call Nuki API and fetch access for my cleaning lady code, then I check on what days she entered the flat and that is a record of her cleaning the flat. Some days she enters multiple times, but I only count the first entrance for that day. I log the date and time.

This module should also keep track of her salary, her salary consist of two items, the cleaning fee and the washing fee. These should be configurable, plus they can change over time, so we should have also a mechanism how to update them and track time when we updated her salary. Once the total salary is calculated by suming the cleaning fee and washing fee, we multiply it by the amount of times she cleaned in the month.

On top of these, we must also make analytics, e.g how many times she cleaned in the month and how much we should pay her for the month.

All the entrance logging, salary counting and monthly statistics should be visible in single page on this module

Additionally I want a heat map of arrival times, e.g when she enters the flat at 09:05, then we count it to 09:00 - 10:00. When she enteres 08:59, it goes to 08:00 - 09:00 and so on. This metrics is taken only from the first entry from that day.

## It needs to keep a log book of all expensess and income for my property - module
This module should mostly work like a simple excel spread sheet where I can log INCOMING and OUTGOING transactions. It should count overall INCOMING and OUTOGING cashflow in total and per month. It should also track total income for the flat. Additionally transaction categories should be supported, so we can get breakdown how much money we spent or received for each category.

There should be some smart functionalities to this tracking, e.g every month I have static expenses such as mortage, utility bill, internet bill etc. These static expenses should be configurable, they are re-occuring and they should be added automatically every month to the log book. They can also change over time, so keep this in mind. Basically this module should act as a financial overview.

Additionally, when I add my cleaning lady salary there as an expense, a margin should be calculated how much money she took from the property income, as I want to track this every month.

## It needs to generate PDF invoices for customers. - module
This one is straight forward, based on the contact details of my property and the customer we generate PDF.
Generation of PDF invoice should be in Slovak or English langunage. It must contain following information:
Dodavatel, this is the information of my property, my name, address, ICO. Variable symbol, e.g invoice number.
Odberatel, is the guest information, name, address, city ZIP code, company name and VAT number.

Then we should state for what period they were staying with us and in what days, then the acutal amount of money they need to pay.

The invoice should also say that it was already paid and there is no need to pay as customers pay via booking.com

These PDF invoices should be stored on the system but I should be also able to download them.

## It needs to sync iCalendar files, ics and display the property occupation. - module
Booking.com offers a http link where ics files of the property occupancy lives. We need to periodically fetch it and display it in a calendar view. On top of these we must have a HTTP endpoint that when called shows these in json format, as I plan to use it further in n8n automation for syncing stuff into google calendar. If you can in easy way sync directly into google calendar then let me know how we could achieve this.

## It needs to create Nuki access codes for the customers. - module
This module works in combination with icalendar file sync module, based on the property occupancy using Nuki API it generates access codes for the occupancy period. But check-in and check-out time is confugurable, so what that means is first day of arrival the code is activi from 14:00 and the check-out time is 09:15 e.g this is the last day of occupancy. The check-in and check-out times must be configurable. This module should also automatically clean up old codes every day. In the UI of this module, I should see all active codes that we have generated and also a history of all old codes, including the time periods for which they were issued.


## It needs to generate mesages for customers from templates that can be copied to clip board - module

Another module that has to work with combination of Nuki access and iCalendar module. This module for each occupancy should offer me a list of text that can be copied to clip board, there will be a text template provided in multiple languages. The service should inject the Nuki access code and check in / check out times into the template. How I envision this is a simple table where each row represents a single entry, then there will be buttons for EN, SK, DE, UE translations of messages for customers with instructions for check in when I click each of the languange button the respective message gets copied to my clip board with all necessary information injected.

# Output
As this is a larger project I want to implement it in modules, so you must go throught the modules one by one. Consider the global requriments first and then design the modular setup. Then the plan is you give me detailed specification for each module but of course follow the first instructions to identify gaps and challenge my approaches.

Also I want you to generate a check list for each implementation feature, so I can cross check with AI develoepr agent, if it actually implemented the features I wanted.
