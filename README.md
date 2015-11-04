------------------
 Swift-consometer
------------------
The goal is to create a small agent that polls a swift cluster
to measure the consumption of each Account in terms of the total 
volume of objects.

This agent needs credentials with `ResellerAdmin` role
to list all the tenants and be able to send a `swift stat`request on
each and all the account.

