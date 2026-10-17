# Choosing guides

Short decision pages for the questions that come up before the how-to guides: which of several similar options fits your case. Each one compares the options in a table, gives "if X then Y" rules, lists the traps, and links every option to its guide and, where one exists, to a runnable example.

| Page | Decides |
|---|---|
| [Which profile, which login](which-profile-or-login.md) | a new home or a login; home, token or API-key login; `--login` vs `AGX_LOGIN` vs a profile or provider default; `-p auto` vs `--login auto` |
| [Which token source](which-token-source.md) | `stored` vs `token_cmd` vs `token_env`; a login vs a profile `secrets:` entry |
| [Running and resuming](running-and-resuming.md) | `run`, `run -p auto`, `new`, `resume`, `ws`; `sessions gc` vs `promote` vs `move` |
| [Library or CLI](library-or-cli.md) | shelling out to `agx --json` vs importing the Go packages, and which package or JSON kind does each job |

Back to the [documentation index](../README.md).
