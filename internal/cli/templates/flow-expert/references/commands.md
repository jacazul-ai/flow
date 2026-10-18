# Commands

Owner of: the index of every command, by topic.

Run `jacazul flow help <command>` for the syntax, prerequisites, effects, and
examples of any command.
{% for topic in topic_list %}
## [{{ topic.Name|capfirst|safe }}]({{ topic.Name|safe }}.md)

| Command | Purpose |
|---|---|
{% for command in topic.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}{% endfor %}
