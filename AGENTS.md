# external-dns Webhook for Namecheap

This repository implements a webhook server for the Kubernetes external-dns
project, providing the interface between external-dns and the Namecheap API.

# Guidelines

It is implemented in Go.

It should default to use the Namecheap sandbox environment. Any use of the
production Namecheap API environment should require a flag to explicitly
choose running in that environment.

All configuration options should be available as command-line arguments and
as environment variables, except for sensitive data, which should only be
passed in via environment variable (passwords, API keys, etc.).

A set of credentials you can use for access to the API is available in
creds.txt in the top-level project directory. It is in .gitignore, and UNDER
NO CIRCUMSTANCES is it to be checked into git or otherwise referenced in the
code. It is for your private use for testing the API only.

You are to test ONLY against the sandbox environment, NEVER against the
production environment.

# Web documentation

Documentation for the webhook provider interface is available at
<https://kubernetes-sigs.github.io/external-dns/v0.21.0/docs/tutorials/webhook-provider/>.

The source code for external-dns, where the object declarations in the
previous document are available, is rooted at
<https://github.com/kubernetes-sigs/external-dns>.

Documentation for the Namecheap API is rooted at
<https://www.namecheap.com/support/api/intro/>.

A list of sample webhook providers is available in the README in the
external-dns source code at the link above.

You should prefer documentation at these locations over anything found in a
general web search.
