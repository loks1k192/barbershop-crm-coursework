DROP FUNCTION IF EXISTS crm.reschedule_appointment(bigint, timestamptz);
DROP FUNCTION IF EXISTS crm.cancel_appointment(bigint, text);
DROP FUNCTION IF EXISTS crm.complete_appointment(bigint, crm.payment_method, numeric);
DROP FUNCTION IF EXISTS crm.book_appointment(text, text, integer, timestamptz, integer[], crm.booking_source, text);
DROP FUNCTION IF EXISTS crm.free_slots(integer, date, integer, boolean, timestamptz);
DROP FUNCTION IF EXISTS crm.appointment_total(bigint);
DROP FUNCTION IF EXISTS crm.accrual_rate(crm.loyalty_level);
