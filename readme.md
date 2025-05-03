# Docker Compose to Zeabur Template Converter

This is a simple tool to convert a docker-compose file to a Zeabur template, powered by AI models (Claude or DeepSeek).

## Usage

Put `docker-compose.(yaml|yml)` in the root directory of the project, and set up your environment:

```bash
# For Claude (default)
echo "CLAUDE_API_KEY=your-api-key" > .env

# For DeepSeek
echo "DEEPSEEK_API_KEY=your-api-key" > .env
```

Then run the tool:

```bash
# Use Claude (default)
go run main.go

# Or specify which AI model to use
go run main.go --ai-model=claude
go run main.go --ai-model=deepseek
```

After the conversion, you will see the Zeabur template in `zeabur-template.yaml`.

You can use the following command to deploy the template to Zeabur:

```bash
npx zeabur template deploy -f zeabur-template.yaml
```
