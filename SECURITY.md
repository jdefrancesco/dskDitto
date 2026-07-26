# Security Policy

## Supported Versions

`DskDitto` is still rather young. Although I started the project sometime around 2020; I have only
worked on it intermittedly. I have a full time job so my open source contributions sometimes become
spotty depending on how busy life is. The version 0.5 is the primary development version. After 
dskDitto is stable, reliable, and time tested we can, as a community release vertsion v1.x.x.
I think this will happen soon as I small community is manifesting and folks want to contribute. 

Now, I generally take security quite seriously as I hold a research position at MIT lab concentrating
on Cyber-Physical systems and Vulnerability Research. That said, for this kind of program secuirty concerns 
I am most worried about and or expect involve dskDitto accidentally removing or mangling important files.
This is why we have the fail-safe code users enter in order to actually perform changes to the file system. 

If anyone does find security concerns of any kind please raise awareness ASAP! 

| Version | Supported          |
| ------- | ------------------ |
| 0.5.x   | :white_check_mark: |


## Reporting a Vulnerability

To report a vulnerability or express a potential security issue please take one or more actions: 

1. Email the report to jdefr89@gmail.com. Please use "DSKDITTO [SEC] - SHORT ISSUE DESCRIPTION" as subject line.
2. Create an official Issue.
3. Opening a Discussion can be helpful for planns on how to properly resolve the bug.

For now I don't have any strict template for a report. Try to keep it short and to the point. 
An example report should include the relevant files/features having issues. I would
probably create the following report:

1. Short Vulnerability Type. Ex: Heap Overflow, Unauthorized File Access. 
2. Version containing bug.
3. A more detailed description of the bug that points out culprit lines of source causing defect.
4. A easy method to replicate and verify your claim.
5. How you think the bug should be remedied.

Again, no hard rules. Just get the point accross, show the issue, help fix it..
I will of course review all reports and ensure they have merit. Not all bugs are vulnerbilities and 
even so, not all vulnerabilities are realistically dangerous or can be exploited in a fashion that causes
any kind of damamge or annoyance. 

Note that these policies are subject to change as `dskDitto` evolves.


