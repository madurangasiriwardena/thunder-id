/*
 * Copyright (c) 2025-2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package session

// Encryptor is the encryption-at-rest seam for sensitive session data (the auth context).
// It is intentionally narrow so a real key-managed implementation can be slotted in later
// without touching callers.
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// passthroughEncryptor is the POC implementation: it does not encrypt.
//
// POC SEAM: this is a no-op. Encryption-at-rest with key management is required before any
// production use — replace this with an implementation backed by the runtime crypto provider.
// TODO(sso): provide a key-managed Encryptor and build key management around it.
type passthroughEncryptor struct{}

// NewPassthroughEncryptor returns the no-op POC encryptor.
func NewPassthroughEncryptor() Encryptor {
	return passthroughEncryptor{}
}

// Encrypt returns the plaintext unchanged.
func (passthroughEncryptor) Encrypt(plaintext string) (string, error) {
	return plaintext, nil
}

// Decrypt returns the ciphertext unchanged.
func (passthroughEncryptor) Decrypt(ciphertext string) (string, error) {
	return ciphertext, nil
}
