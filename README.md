# EdgeGuard

`EdgeGuard` is an extension of the Datasance PoT  and the Eclipse ioFog Hardware Abstraction Layer Microservice. It serves as a critical security and monitoring component for edge devices running ioFog Agents.

The primary function of EdgeGuard is to monitor the Hardware Abstraction Layer (HAL) Microservice REST API, collecting comprehensive hardware details from endpoints such as `lscpu` , `lspci` , `lsusb` , `lshw` and `/proc/cpuinfo`. Using this data, EdgeGuard generates an irreversible hardware signature unique to the edge device.

In the event of a detected hardware signature change, EdgeGuard immediately deprovisions the ioFog Agent from the Edge Compute Network (ECN) cluster. This action ensures that unauthorized changes or tampering with edge devices are swiftly addressed, maintaining the integrity and security of the ECN.

EdgeGuard is particularly valuable for edge devices deployed outside traditional datacenters, where the risk of unauthorized access is significantly higher. By detecting potential security breaches and wiping all microservices running on the compromised ioFog Agent, EdgeGuard helps organizations adhere to strict security policies and minimize exposure to potential threats

