# Plan: Default UserName to APIUser, Make ClientIp Optional

## Summary

Based on Namecheap API documentation:
- **UserName**: Docs state "Generally, the values of ApiUser and UserName parameters are the same." All official SDKs default them to the same value.
- **ClientIp**: The Terraform provider made this optional with auto-detection. We'll make it optional without auto-detection for now.

## Changes

### 1. `config.go`

**Line 36** - Update username flag description:
```go
// Before:
flag.StringVar(&cfg.Username, "username", "", "Namecheap username (env: NAMECHEAP_USERNAME)")
// After:
flag.StringVar(&cfg.Username, "username", "", "Namecheap username (env: NAMECHEAP_USERNAME, defaults to api-user)")
```

**Line 37** - Update client-ip flag description:
```go
// Before:
flag.StringVar(&cfg.ClientIP, "client-ip", "", "Client IP for Namecheap API (env: NAMECHEAP_CLIENT_IP)")
// After:
flag.StringVar(&cfg.ClientIP, "client-ip", "", "Client IP for Namecheap API (env: NAMECHEAP_CLIENT_IP, optional)")
```

**Lines 115-120** - Remove Username and ClientIP from validation:
```go
// Remove these blocks from Validate():
if c.Username == "" {
    return fmt.Errorf("username is required (set via --username or NAMECHEAP_USERNAME env var)")
}
if c.ClientIP == "" {
    return fmt.Errorf("client-ip is required (set via --client-ip or NAMECHEAP_CLIENT_IP env var)")
}
```

### 2. `namecheap.go`

**`NewNamecheapClient()` function** - Default username to APIUser:
```go
// Before:
func NewNamecheapClient(cfg *Config) *NamecheapClient {
    return &NamecheapClient{
        ...
        username: cfg.Username,
        ...
    }
}

// After:
func NewNamecheapClient(cfg *Config) *NamecheapClient {
    username := cfg.Username
    if username == "" {
        username = cfg.APIUser
    }

    return &NamecheapClient{
        ...
        username: username,
        ...
    }
}
```

### 3. `namecheap_test.go`

Add a test verifying UserName defaults to APIUser when not explicitly set.

### 4. `README.md`

Update documentation:
- Credentials table: mark `NAMECHEAP_USERNAME` as optional (defaults to API_USER), mark `NAMECHEAP_CLIENT_IP` as optional
- Flags table: update `--username` and `--client-ip` descriptions
- Docker/Kubernetes examples: mark as optional rather than required

## Behavior Changes

- **UserName**: If not set, defaults to the value of `APIUser`
- **ClientIp**: No longer required; users can omit it
