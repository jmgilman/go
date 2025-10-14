package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test SSH key fixtures (generated for testing only, not real keys).
const (
	// testSSHPrivateKey is a test RSA private key (unencrypted).
	testSSHPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAybOnlFK1K69MkKKff+UioLGkqhGO8Q1pRiqXhl4a0zxF8+ex
r0lIhloAN60a1moFyW5YSu2tQRklI0qRSMBKqDHwnx06/yenw0JORT9kNQuEGgaT
qNMzuQkVKhd7AOBnuUvMvy7WCO4QD5SryIEj3solPKHAR6zyCFWt3toyVCR0QjFR
3inl5XVhPqnq2xccCDmGnNAUWh6wrVzXhY02wN8qS0LcBnFRSsyg0Y9d6Ca30Vc/
xqoY89Pa0Qklm612gTeXwG1ChLPfIbm4ABR4pZ+33au3Epe4K3pYsTKt1vYGnmHS
0qXCM/3UzuRwOk/wKcHu/48R5glkUrga3EB2cQIDAQABAoIBAFtaznTkfQAbNq9v
qJQxwNxNeUo6B6bwLxVDpzuJldbEvt44u4arx3hqfRy6f6RLgvF30++j9Mu+Ss7Q
MDtmNKo3bEd04sq8OES83FyK2KUZ4Sw0fF6DwjJ1hat51RFRkkkfps2UtgZ3ZLjZ
2nBG5Ws73V+31zHfiAP0YnrEEvV+etft20/fKngpWbKSZX4bwee3/7mBL9ZGJmXk
cRcwa2GIYjLVq0ydphTzhoSQpAPJTaG4aj4y6odvWbvcvtAXvDXADAT8sIax/HUd
sf0GwiiiGK7WRGmxEu/agDAw3fzsd2xBMNqkbm2NgE/tQpBZyZRZ19PdiECzlg4i
NvdpWR0CgYEA9gSxGsdrGEWVFWHcwFcsZ9yUNYfMmtRPa4mjvBWmuuIfNyYozCoC
HgBqNP6bJvWJehOQqzTW9KwD5pO2z3gImlXRKF3iBstY4aWP22zdVmPvSaafiomp
ubHWvfukAHmXsQuF+2dwkT18KxpxNgdp+mcdfbohfqD8e9fwRnUnNXMCgYEA0eKp
dI7I/pDGudwPFRIN8XgV0a1ZJ+NeMpIZsVVgLcRyfLY+e18GxybfTtqKyL/OBH3t
qq+jX4cOet/Na0VEG7Fyrw8fN1BJq8I6ND7CFvAbsLXRqdcQKKQKs2mO3Yr0GFna
uCgSjgeXmJlQBtOEbTDTKxRGK0hUQHXPgS4Li4sCgYEA7Y+SRT2TmJh4YXFibQjA
hHpnU1mSpV+mYT1DsndlzMhVRDfA5YUbDkVwSUQiJfirjAoghHI9r337NkglGynZ
hM6hbc1aWR068omg5E23XZialBAltu0/y2SC7Gl18E95vyhVdHJqLJWmtSiPcZCv
MXEo9SMq/NAPfrcB+cde2SkCgYBms9ghvgDieGuV5PXIZLZH83AR0xZua1bbvhwu
Z02R96/iELeQXRaO+xmIl24T/69LCWfz/tAd3ZObUspM9G74ciNhQDARPAtgrcEX
caI94S5bkQzQY/l3OZY25q9O/0CkbcuWE53IvDRVKqg7PuNtHtgmG1yer1zy0fNB
Dgv+MwKBgBQgSHSYnGBr58ZOYfXNAlQyZJHYo5Uuzg5egUGMJY6BOv2MvoD6EmmT
Z7kgDnzMAbEJ2JJWtGh/5lDzquFkB8t/FTtqraynTwKbY+53lx+O5b7lq3VIPFwV
SpZXEAA7PaXL9zRMDNoenMOHo1ih/AcY28cq9jUV2osGv5xxEjVp
-----END RSA PRIVATE KEY-----`

	// testSSHPrivateKeyEncrypted is a test RSA private key (encrypted with password "testpass").
	testSSHPrivateKeyEncrypted = `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: AES-128-CBC,EAEC9661134A62F3CBF8A65031931350

celyLYwKj3sQP2pr+KHyen5ROiprWW/CSAAxfAjtrxNa5ZCSu/8nXHzXRMiiEqyS
LjsA0ZrQh16atyH97+LgzL7MjEukzPLNe494xdRbZaOhH3OZX/CilBx/RiL54soa
ChvLBzkzEk0nbErXSYLCoN3MzOBVkCOxRZ5gsw6f7o1lnflkfDyAaVjnf3OxfduI
ghco6tO1rl8yswK70UuLrrU2RmY6h/leWoxqzCqnWaJlN3S0VDj6/w5imV4uyBZo
aiqBw4RTtXf98460XkItjPV26jO7P0So8Hg5su3cUal+71I6NqMe8Ns5qAv1nrty
ad8mG1vS5LF6Y+QZPocl1x0By4eacXVB3jo7P27mo1tVrvW/dImbBD88eOzhxSoE
bpupyOGZYwOnBX7Z78CKdILAWSLcjZuCmGSIm8AAEAts4pH8UDebMl8TawGKN04P
rVM7au/icv2zxGcxwXB1TssxAjMX7ujQ0wmAMW2VlH+/tbd2zj1mm/fW7iIdu/lM
ktd5QA6ZhfeBzI0xFvaAScDObCy9OZDJj+G+4p+WMEh7oDV187L+KtKTKcicIhnv
UmXFD7DUF7aL5cQSkoQ2pslVNoX4bcdY7gTRYEYaJv473ktlS4d4lcOatV7ccVS0
XfmNFz4gaj89gUiCqH39+olSd2SNO+K61z0NlCf9U69XAO80qQZEVu1e9kFh8qvn
rXxIKhIA20LhCGhmbBkXQ8iWpe3TapKfit9dQXKI6n0Yr84N5F5b6lgqKvwkk5iV
MRwg0/TVdjlSkEApape+wl67d9uBlqOoTb8qtaKbIcNYwQsd9GSKoDGElP9Pop7P
fL04RPSZqK1NA63nJtm1s0snyZLcHINR1CyGMGtOIuvFvaeoasGD45L4bM3da8VR
OQVpswWfGpf+A8OAbrhnpJVsfJcotsb15E+W8xFPgfJovxW4ibpRbj+e+GxyLOkA
9r3S5D55gAoBVKMR1/aXidkuEBWfWTOqgpYJho7lMhD9TMcG6nmn5wSIWqf7v4G8
1EbK1llGJAjc/4Hztx1lhinCeTCINcUV+ggFJNxT1mReFbC0/O59vfUYqydYusNi
aRqyiN3xOTbnVvG76+eKaIRR2WtLm/4NODKs90CxEoxLdTiSNSVzv2yc8eqt9wjo
9a477/nIKV1fBvmazkQPXatf3PT6qNqk2MisORMA13DW/ckl6+5cYspK3DAtRM3d
qDoOnQO2ykuNGoccbidFXZWqSeh9FFcOyJL8uyGRPLcqscoXZ55z7ITeM/L5pW4G
lF9R8NlH/fELotXQUS8EsJ1ROQMSPATkqzNhNPOq8+2QVYiWsa9Qd/4GRhXMKe2n
ulbCbGQDfGJZCRhuuw0k0DDBavfGp6Orqnp9FW0xifqbQevpRquDuOP0F39fEFfY
HxWeEnzp931lXU0FnaS/yAHxqrNzs6SHJZ/6oR+4F+4+7kG7pQWie1sAG5uhGhq4
QkxAIN9xHnDBWJ7CbwKGQql8PovHfgKQy6Pf0bdThRxiyLZf7hAozy0jIL6mYaK8
sNL/sWx5AW8bkK4dSsXuk4N241M612dTKTP07EfPNnFCRR4dfyxPvt2AKq/+MeYV
-----END RSA PRIVATE KEY-----`
)

// TestSSHKeyAuth tests SSH authentication from PEM bytes.
func TestSSHKeyAuth(t *testing.T) {
	t.Run("valid unencrypted key", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte(testSSHPrivateKey))
		require.NoError(t, err)
		require.NotNil(t, auth)

		// Verify it implements transport.AuthMethod
		_, ok := auth.(transport.AuthMethod)
		assert.True(t, ok, "Auth should implement transport.AuthMethod")
	})

	t.Run("valid encrypted key with correct password", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte(testSSHPrivateKeyEncrypted), WithSSHPassword("testpass"))
		require.NoError(t, err)
		require.NotNil(t, auth)

		// Verify it implements transport.AuthMethod
		_, ok := auth.(transport.AuthMethod)
		assert.True(t, ok, "Auth should implement transport.AuthMethod")
	})

	t.Run("encrypted key with wrong password", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte(testSSHPrivateKeyEncrypted), WithSSHPassword("wrongpass"))
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to parse SSH key")
	})

	t.Run("encrypted key with no password", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte(testSSHPrivateKeyEncrypted))
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to parse SSH key")
	})

	t.Run("invalid PEM bytes", func(t *testing.T) {
		invalidPEM := []byte("not a valid PEM key")
		auth, err := SSHKeyAuth("git", invalidPEM)
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to parse SSH key")
	})

	t.Run("empty PEM bytes", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte{})
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to parse SSH key")
	})

	t.Run("different username", func(t *testing.T) {
		auth, err := SSHKeyAuth("customuser", []byte(testSSHPrivateKey))
		require.NoError(t, err)
		require.NotNil(t, auth)
	})
}

// TestSSHKeyFile tests SSH authentication from key file.
func TestSSHKeyFile(t *testing.T) {
	t.Run("valid key file", func(t *testing.T) {
		// Create temporary key file
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "id_rsa")
		err := os.WriteFile(keyPath, []byte(testSSHPrivateKey), 0600)
		require.NoError(t, err)

		// Test reading the key
		auth, err := SSHKeyFile("git", keyPath)
		require.NoError(t, err)
		require.NotNil(t, auth)

		// Verify it implements transport.AuthMethod
		_, ok := auth.(transport.AuthMethod)
		assert.True(t, ok, "Auth should implement transport.AuthMethod")
	})

	t.Run("encrypted key file with password", func(t *testing.T) {
		// Create temporary key file
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "id_rsa_encrypted")
		err := os.WriteFile(keyPath, []byte(testSSHPrivateKeyEncrypted), 0600)
		require.NoError(t, err)

		// Test reading the key with correct password
		auth, err := SSHKeyFile("git", keyPath, WithSSHPassword("testpass"))
		require.NoError(t, err)
		require.NotNil(t, auth)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		auth, err := SSHKeyFile("git", "/nonexistent/path/to/key")
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to read SSH key file")
	})

	t.Run("file with invalid content", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "invalid_key")
		err := os.WriteFile(keyPath, []byte("not a valid key"), 0600)
		require.NoError(t, err)

		auth, err := SSHKeyFile("git", keyPath)
		assert.Error(t, err)
		assert.Nil(t, auth)
		assert.Contains(t, err.Error(), "failed to parse SSH key")
	})
}

// TestBasicAuth tests HTTP basic authentication.
func TestBasicAuth(t *testing.T) {
	t.Run("valid credentials", func(t *testing.T) {
		auth := BasicAuth("username", "password")
		require.NotNil(t, auth)

		// Verify it implements transport.AuthMethod
		_, ok := auth.(transport.AuthMethod)
		assert.True(t, ok, "Auth should implement transport.AuthMethod")

		// Verify it's the correct type
		basicAuth, ok := auth.(*http.BasicAuth)
		require.True(t, ok, "Auth should be *http.BasicAuth")
		assert.Equal(t, "username", basicAuth.Username)
		assert.Equal(t, "password", basicAuth.Password)
	})

	t.Run("token-based auth", func(t *testing.T) {
		auth := BasicAuth("mytoken", "x-oauth-basic")
		require.NotNil(t, auth)

		basicAuth, ok := auth.(*http.BasicAuth)
		require.True(t, ok)
		assert.Equal(t, "mytoken", basicAuth.Username)
		assert.Equal(t, "x-oauth-basic", basicAuth.Password)
	})

	t.Run("empty credentials", func(t *testing.T) {
		// Even empty credentials create a valid Auth object
		auth := BasicAuth("", "")
		require.NotNil(t, auth)

		basicAuth, ok := auth.(*http.BasicAuth)
		require.True(t, ok)
		assert.Equal(t, "", basicAuth.Username)
		assert.Equal(t, "", basicAuth.Password)
	})
}

// TestEmptyAuth tests nil authentication for public repos.
func TestEmptyAuth(t *testing.T) {
	t.Run("returns nil", func(t *testing.T) {
		auth := EmptyAuth()
		assert.Nil(t, auth)
	})
}

// TestAuthInterface verifies Auth interface compatibility.
func TestAuthInterface(t *testing.T) {
	t.Run("SSH auth satisfies Auth interface", func(t *testing.T) {
		auth, err := SSHKeyAuth("git", []byte(testSSHPrivateKey))
		require.NoError(t, err)

		var _ = auth
	})

	t.Run("Basic auth satisfies Auth interface", func(_ *testing.T) {
		auth := BasicAuth("user", "pass")
		var _ = auth
	})

	t.Run("nil satisfies Auth interface", func(t *testing.T) {
		auth := EmptyAuth()
		assert.Nil(t, auth)
	})
}
