---
layout: home
hero:
  name: TechPedia
  text: A multilingual Go wiki
  tagline: Write connected documents. Run one small service. Keep editorial control.
  actions:
    - theme: brand
      text: Start locally
      link: /getting-started
    - theme: alt
      text: Deploy a server
      link: /deployment
features:
  - title: Documents first
    details: TechMark source, revision history, internal links, media, and reusable display boxes.
  - title: Connected languages
    details: Independent documents per language, linked by central Techdata identity.
  - title: Simple deployment
    details: Go and BadgerDB, with the Vue frontend embedded in the server binary. No Node runtime in production.
---

TechPedia is a topic-independent wiki engine maintained by Wave Foundation.
An installation can focus on computer science or another subject without changing
the engine. It is not a general CMS, blog, or social-network platform.

This site documents the implemented engine. It does not publish an installation's
articles, images, user accounts, credentials, or database. Administrative UI text
remains independent of this English documentation; public interface messages are
configured per language edition.

Start with the [local setup](getting-started.md), [language-edition setup](language-editions.md),
and [editing guide](editing.md). Operators should also read
[security](security.md), [configuration](configuration.md), and
[backup and recovery](operations.md).
