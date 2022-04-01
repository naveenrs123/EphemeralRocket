# The Ephemeral Rocket

**Proposal:** https://docs.google.com/document/d/1hNxcKpOs3o37wjD0Q9mV1JGZ5ZqUWA4hAxYnUHBV6O8/edit?usp=sharing

## Milestone 1
**Happy Path**
- Add necessary structs (Server, Coord, Client) with internal data structures
- Implement MessageLib API (for Client)
- Servers can join the system
- Clients can join the system and be assigned a primary and 2 secondaries

**Deliverable Document:** https://docs.google.com/document/d/15uG1hbPTbNWCGt8Kr0GwcpzDVVB9nkiPw5xT94j1xe8/edit?usp=sharing

## Milestone 2
**Finish Happy Path + Basic Failure Handling**
- Clients can successfully send messages to other clients
- Client can leave and re-enter the system
- Add fcheck so coord can monitor all the servers
- Secondary server failure
- Primary server failure

## Milestone 3
**Finishing Failure Handling + Debugging/Testing**
- Routing server failure
- Multiple server failure
- Run scenario tests
- Ensure system runs and debug any failures from the previous sections!
