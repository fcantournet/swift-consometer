------------------
 Swift-consometer
------------------
The goal is to create a small agent that polls a swift cluster
to measure the consumption of each Account in terms of the total
volume of objects.

This agent needs credentials with `ResellerAdmin` role
to list all the tenants and be able to send a `swift stat`request on
each and all the account.


# Hacking

To deploy on lab0 for testing purposes just do :
+`make deploy`

This will compile and deploy on bstcld on lab0 in your home directory.
You'll need a test .yaml file for credentials and config
