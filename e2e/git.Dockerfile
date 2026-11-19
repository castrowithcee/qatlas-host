# Git platform of one environment: the base image from deps with the SSH settings and three seeded demo stacks
# repositories.
ARG BASE
FROM ${BASE}

# qhost-deploy is a further name for the account git, so the deploy service reaches the repository by a URL
# without a user part.
RUN useradd -o -u "$(id -u git)" -g git --home-dir /srv/git --no-create-home --shell /usr/bin/git-shell qhost-deploy \
	&& usermod -p '*' qhost-deploy
COPY git-sshd.conf /etc/ssh/sshd_config.d/qhoste2e.conf
COPY demo-stacks /usr/local/share/demo-stacks
COPY demo-stacks-second /usr/local/share/demo-stacks-second
COPY demo-stacks-root /usr/local/share/demo-stacks-root
COPY git-entrypoint.sh /usr/local/bin/git-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/git-entrypoint.sh"]
