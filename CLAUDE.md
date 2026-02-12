# Instruções para o Claude

## Fonte da verdade: MCP da Pomelo

**Sempre consulte o MCP da Pomelo antes de qualquer decisão sobre contratos de API, tipos de transação, campos obrigatórios, comportamentos esperados e cenários.**

O MCP expõe a documentação oficial da Pomelo. Use as ferramentas disponíveis:

- `list_topics` — lista todos os tópicos disponíveis
- `list_endpoints_by_topic` — lista endpoints de um tópico (ex: `transactions`)
- `get_endpoint` — detalhe completo de um endpoint (campos, respostas, regras)
- `search_endpoints` — busca por intenção em linguagem natural
- `generate_request_example` — gera exemplo de código para um endpoint

**Fluxo obrigatório:** ao adicionar, modificar ou validar qualquer cenário do simulator ou request do Postman, primeiro consulte o MCP para confirmar o contrato esperado pela Pomelo, depois aplique as mudanças.
