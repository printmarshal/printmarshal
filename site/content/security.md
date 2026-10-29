---
title: "Security with explicit control"
description: "Read-only discovery, purpose-bound credentials, dry-run write gates, audit records, certificate enrollment, and clear implementation limits."
---

## Read first. Change deliberately.

PrintMarshal discovers and monitors printers through read operations. Assessments inspect reported settings, TLS behavior, and reachable ports. They do not remediate a device automatically.

A device-changing workflow requires an authorized action, suitable credentials, and an implemented driver capability. Role checks distinguish viewers, operators, and administrators. A configured campaign can continue under its saved authorization; there is no separate browser confirmation for every device write.

## Write gates and dry runs

Configuration apply and revert, security remediation, credential rotation, certificate enrollment, and firmware start default to dry run. Only explicit `dryRun:false` selects real execution. The API does not require proof of a previous successful rehearsal.

Dry run previews supported behavior without device-changing requests. Discovery, polling, probes, and previews can still contact devices for reads. Installing an already-issued certificate has no preview flag; removing a local print-provider queue is also an immediate authorized action. Those exceptions are explicit in the API documentation.

## Separate read and write credentials

Device credentials carry a purpose: `read`, `write`, or `both`. Existing credentials default to read-only purpose. Real write workflows resolve write-capable credentials and refuse devices with none bound. Marking a credential as writable does not grant permission on the printer itself.

Stored device secrets and issued private keys use AES-256-GCM encryption. The encryption key comes from `PM_SECRET_KEY` or the data directory's `secret.key`. Preserve that key with your backups; replacing it cannot recover existing ciphertext. Device secrets are not returned through the credential API.

User passwords use bcrypt. Session and API-token secrets are stored as SHA-256 hashes. External-provider secrets are supplied through referenced environment variables.

## Audit and verification

Write workflows retain the actor, target, and result in audit and job records. Configuration writes include per-setting outcomes; a fresh read can establish that a change was verified, diverged, or could not be verified. Security remediation uses reassessment, and firmware checks the reported version after delivery.

Acceptance is not proof that the desired state took effect. Certificate installation currently checks the driver's error rather than independently re-probing the installed certificate. Audit persistence failures are logged and need investigation; they do not imply the device stayed unchanged. Eligible audit and security events can be exported to a SIEM.

## Assess policy without inventing evidence

Define desired security settings and compare them with driver data, TLS observations, and port probes. Missing observations remain unknown and are excluded from the scored pass/fail denominator. An all-unknown result is not reported as compliant.

Remediation defaults to preview and offers only changes that an implemented writer can perform. The management-path rule skips a tagged change that would remove the final known management path. It is a workflow safeguard based on the capability profile, not a guarantee of recovery from every network or device failure.

## Certificates and enrollment

SCEP and EST clients generate keys and certificate requests, obtain certificates from a configured CA, and retain the private key encrypted at rest. Observed certificate expiry produces alerts. Enrollment dry runs generate a request and report the plan without contacting the CA.

SCEP supports initial RSA enrollment, not renewal requests. EST implements CA-certificate retrieval and simple enrollment, not client-certificate authentication or re-enrollment. ACME supports DNS-01 with manual DNS publication; there is no automatic renewal scheduler.

Issuing and installing are separate operations. No shipped real-vendor driver installs certificates. Issued public certificate data is available through the API, but the private key is not downloadable, so the current API does not supply a manual certificate-and-key installation bundle.

## Rotation stages before device changes

The rotation workflow encrypts and stores the new device-scoped credential before asking a writer to change the device. Dry runs stage nothing. A staging failure leaves the device untouched.

Clean refusal removes the staged credential. Ambiguous per-setting failures can retain it because the device may already be using the new secret; a whole-driver error without setting results takes the removal path. Staged entries participate in normal credential resolution. Review the retained credential and device state before relying on or deleting it. No shipped real-vendor security writer provides these rotations out of the box.

## Transport and session controls

Deploy the HTTP service behind an HTTPS reverse proxy or ingress and set `PM_SECURE_COOKIES=true`. Session cookies are HttpOnly and SameSite=Strict. Failed sign-ins are throttled, password changes revoke other sessions, and live streams recheck authentication periodically. OIDC and SAML validate their protocol state; LDAP can use LDAPS or StartTLS. RADIUS is UDP PAP, without native RadSec or challenge-response prompts.

IPP writes try TLS endpoints first and pin the presented device certificate on first TLS contact. Pins live only for the process lifetime. Set `PM_IPP_WRITE_REQUIRE_TLS=true` to refuse plaintext writes. Device TLS certificate validation is skipped by default through `poll.ippInsecureTls:true`; first-contact pinning does not replace CA verification or protect the initial connection from an attacker already in the path.

## Know the shipped boundary

There are no shipped real-vendor firmware updaters, security writers, or certificate installers. Firmware delivery is limited to the simulator transport and its explicit allow-list. Standards SNMP and IPP can write a limited configuration setting set on devices that support it.

API-token group scope filters defined device operations; it is not tenant isolation for every collection or event. The server runs as one active process, without a distributed worker lease. These limits are part of deployment planning, not features hidden behind an edition.

[Read the full security guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Security.md) · [Review identity and scope](https://github.com/printmarshal/printmarshal/blob/main/wiki/Identity-and-access.md) · [Deploy PrintMarshal](/get-started)
