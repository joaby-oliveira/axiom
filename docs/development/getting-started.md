# Getting Started

## Clone the repository

Em um diretório de sua escolha:

```bash
git clone https://github.com/rgomids/axiom.git
cd axiom
```

Os comandos seguintes assumem a raiz do repositório. Se já houver um checkout,
use-o sem mover, apagar ou sobrescrever outros diretórios. SSH não é requisito.

## Minimum requirements

- Git; acesso HTTPS ao GitHub para clonar o repositório público;
- Bash;
- utilitários POSIX usados pelos scripts (`find`, `grep` e `awk`);
- Codex para o caminho Runtime E2E; não é necessário para checks Go/shell;
- GitHub CLI autenticado para operações reais de Work Item (o dogfood
  determinístico usa um fake controlado).

A fundação Go da Specification 002 contém domínio, contratos de aplicação e codecs
portátil e local JSON. Go 1.26 ou posterior é requisito para os checks Go; o codec
portátil depende de `go.yaml.in/yaml/v3@v3.0.5`; o store POC usa
`golang.org/x/sys/unix@v0.44.0` em Linux/macOS.
Consulte [preparação do cache e validação offline](../commands.md#t01t04-validation)
e [Evidence T04](../specifications/002-lingo-project-initialization/evidence-t04.md).
O POC Lingo oferece lifecycle portátil/local, bindings e resolução global de
Project, bootstrap do Codex, Work Items GitHub e workflow sequencial persistente.
Paths absolutos permanecem no estado local; credenciais continuam no `gh`.
Recuperação automática e a matriz completa de falhas/races não são prometidas;
#19/#20 permanecem abertas e #21 aguarda aceite humano. Consulte
[comandos](../commands.md).
Consulte o [lifecycle atual](../specifications/README.md#002--lingo-project-initialization)
para distinguir implementação mergeada, aceitação humana e autorização de novas Tasks.

## Validate the harness

Na raiz do repositório:

```bash
./scripts/validate-repository.sh .
./scripts/validate-agent-package.sh .
```

Execute também os checks locais de segurança:

```bash
./scripts/test-validate-agent-package.sh
./scripts/test-check-sensitive-files.sh
./scripts/check-sensitive-files.sh .
```

## Work with Codex

1. Execute `./scripts/install-axiom.sh`.
2. Se o installer informar `pathConfigured: false`, execute a instrução exibida;
   para o destino padrão: `export PATH="$HOME/.local/bin:$PATH"`. O installer não
   altera arquivos de shell/profile.
3. Confirme `command -v axiom` e `axiom version` fora do checkout.
4. Execute `axiom first-run`. Ele detecta `codex` e `claude` no `PATH` e instala
   ou atualiza as skills globais do Axiom para cada Runtime encontrado; sem
   Runtime, apenas informa e termina com sucesso. Repetir é idempotente.
5. Configure um Project com `axiom project configure`; revise o preview e confirme
   a mesma proposta. O modo completo exige `--project-id`, `--preview-digest` e
   `--authorize-local` na segunda chamada.
6. Inicie Codex fora do repository e invoque uma skill global `$axiom-*` usando
   somente seletores lógicos.
7. Preserve a separação entre discovery, hipótese, Specification, decisão,
   implementação e Evidence; não invente comandos futuros.

## Before a commit

Siga [../security/repository-security.md](../security/repository-security.md). No mínimo, revise todo o diff staged, execute o checker com `--staged`, valide o harness e confirme que nenhum dado privado de providers foi incorporado.

Lista consolidada de comandos: [../commands.md](../commands.md).
