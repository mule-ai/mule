import json

# Read the system prompt from the JSON config file
with open('godeps_fix_agent_config.json', 'r') as f:
    config = json.load(f)
    system_prompt = config['system_prompt']

# Prepare the update payload
payload = {
    "name": "code-editor",
    "description": "Expert Go dependency management specialist",
    "provider_id": "aa2a0b26-01dd-45c4-898f-f9c27243273b",
    "model_id": "nanogpt/qwen/qwen3-coder",
    "system_prompt": system_prompt
}

# Write the properly formatted JSON to a file
with open('agent_update_payload.json', 'w') as f:
    json.dump(payload, f, indent=2)

print("Payload created successfully")