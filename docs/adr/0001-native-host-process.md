# Backend runs as a native host process, not in a container

Nexus's Files module reads and mutates the Host filesystem and its Servers
module reads Host vitals (CPU, memory, disk, network, Docker socket). We run the
Go backend as a native process directly on the Host so it has first-class access
to the filesystem and `/proc`-style metrics. Only Postgres is containerized (a
one-service docker-compose). Containerizing the backend was rejected because it
would require privileged mounts of host directories, `/proc`, `/sys`, and the
Docker socket, which is dirtier and less secure than just running on the Host.
