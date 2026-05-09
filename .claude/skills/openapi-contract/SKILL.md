---
name: openapi-contract
description: Generate and maintain OpenAPI 3.1 spec for Obscura API. Source of truth for endpoint contracts. Used by client SDK generation, docs, contract tests.
---

# OpenAPI 3.1 Contract

## Location

`backend/api/openapi.yaml` — source of truth.

Generated artifacts (gitignored):
- `frontend/lib/api-types.ts` — TypeScript types
- `mobile/lib/api-types.ts`
- `docs/api/index.html` — Swagger UI

## Top of file

```yaml
openapi: 3.1.0
info:
  title: Obscura API
  description: |
    Privacy-first federated messaging platform.
    Spec v3.0. See docs/spec/obscura_spec_v3.txt for protocol details.
  version: 1.0.0
  contact:
    email: dev@obscura.network
  license:
    name: TBD

servers:
  - url: https://api.obscura.network
    description: Production
  - url: http://localhost:8080
    description: Local development

security:
  - bearerAuth: []

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

  schemas:
    Envelope:
      type: object
      required: [success]
      properties:
        success: { type: boolean }
        data: { description: "Response payload (success only)" }
        error: { type: string, description: "Error message (failure only)" }

    User:
      type: object
      properties:
        did: { type: string, pattern: "^did:obs:[a-f0-9]{64}$" }
        username: { type: string }
        display_name: { type: string }
        avatar_url: { type: string, format: uri }
        tier: { type: integer, minimum: 1, maximum: 5 }
        created_at: { type: integer, format: int64 }

    Conversation:
      type: object
      properties:
        id: { type: string, format: uuid }
        is_group: { type: boolean }
        members: { type: array, items: { type: string } }
        last_message_at: { type: integer, format: int64 }

    Message:
      type: object
      properties:
        id: { type: string, format: uuid }
        conversation_id: { type: string }
        from_did: { type: string }
        ciphertext: { type: string, contentEncoding: base64 }
        message_type: { type: string, enum: [text, image, voice, file, location, call_invite, call_accept, call_end, group_invite, zk_proof] }
        created_at: { type: integer }
        deleted_at: { type: integer, nullable: true }

    PreKeyBundle:
      type: object
      properties:
        identity_key: { type: string, contentEncoding: base64 }
        signed_prekey:
          type: object
          properties:
            id: { type: integer }
            key: { type: string, contentEncoding: base64 }
            signature: { type: string, contentEncoding: base64 }
        one_time_prekey:
          type: object
          properties:
            id: { type: integer }
            key: { type: string, contentEncoding: base64 }

    ZKProofVerifyRequest:
      type: object
      required: [proof_json, circuit_id, public_inputs]
      properties:
        proof_json: { type: string, description: "snarkjs proof JSON" }
        circuit_id: { type: string, enum: [credit_threshold, identity_proof, message_integrity, token_balance, vote_proof, storage_proof] }
        public_inputs: { type: array, items: { type: string } }
```

## Endpoint definition pattern

```yaml
paths:
  /v1/auth/request-otp:
    post:
      summary: Request SMS OTP for phone verification
      tags: [auth]
      security: []   # public endpoint
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [phone]
              properties:
                phone:
                  type: string
                  pattern: "^\\+[1-9][0-9]{6,14}$"
                  example: "+905551234567"
      responses:
        '200':
          description: OTP sent
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Envelope'
                  - type: object
                    properties:
                      data:
                        type: object
                        properties:
                          ttl_seconds: { type: integer, example: 300 }
        '400':
          description: Invalid phone
        '429':
          description: Rate limited
```

## Generate TypeScript types

```bash
npx openapi-typescript backend/api/openapi.yaml -o frontend/lib/api-types.ts
npx openapi-typescript backend/api/openapi.yaml -o mobile/lib/api-types.ts
```

## Generate Swagger UI

```bash
npx @redocly/cli build-docs backend/api/openapi.yaml -o docs/api/index.html
```

## Contract tests

Use Pact or schemathesis to verify backend matches spec:

```bash
pip install schemathesis
schemathesis run http://localhost:8080 --base-url http://localhost:8080 \
  --spec backend/api/openapi.yaml --hypothesis-deadline 5000
```

## CI integration

```yaml
# .github/workflows/api-contract.yml
- name: Validate OpenAPI spec
  run: npx @redocly/cli lint backend/api/openapi.yaml

- name: Check generated types match committed
  run: |
    npx openapi-typescript backend/api/openapi.yaml -o /tmp/api-types.ts
    diff /tmp/api-types.ts frontend/lib/api-types.ts

- name: Schemathesis fuzz
  run: schemathesis run ...
```

## Rules

- ALL endpoints must be in openapi.yaml — undocumented endpoints fail CI
- Response shape always Envelope (`{success, data | error}`)
- DID pattern enforced via regex
- Phone E.164 enforced
- Authentication explicit per endpoint (`security: []` for public)
- Examples in every request/response
- Versioned (path prefix `/v1/`, breaking changes → `/v2/`)
- Diff against generated types committed = breaking change requires deliberate update
