/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const pt: AppCatalog<typeof en> = {
  'skip.content': 'Saltar para o conteúdo',
  'nav.products': 'Produtos',
  'nav.sovereignty': 'Soberania',
  'nav.standards': 'Normas abertas',
  'nav.pricing': 'Preços',
  'footer.pricing': 'Preços',
  'home.pricing.title': 'Três planos, a suite completa',
  'home.pricing.body':
    'Os mesmos produtos nos três: o que muda é o armazenamento por utilizador. Sem edições reduzidas.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'sem IVA',
  'home.pricing.per': '/Utilizador/Mês',
  'home.pricing.cta': 'Falar com a equipa comercial',
  'home.pricing.commitment.title': 'Um compromisso para toda a vida',
  'home.pricing.commitment.body':
    'O preço por utilizador sobe 0,50 € a cada quatro anos, e mais nada: para toda a vida. Fica a sabê-lo no dia em que adere e não muda.',
  'home.pricing.included.title': 'Uma única edição',
  'home.pricing.included.body':
    'Sem planos por funcionalidade, sem escalões, sem edições reduzidas: a suite completa está incluída no preço por utilizador, com as mesmas funcionalidades para todos.',
  'home.pricing.metered.title': 'O que é faturado à parte',
  'home.pricing.metered.body': 'O armazenamento adicional: por utilização ou em blocos.',
  'home.pricing.storage.title': 'O armazenamento é partilhado',
  'home.pricing.storage.body':
    'A quota de cada pessoa soma-se à da organização: o que uma pessoa não usa fica disponível para o resto da organização. E a quota por utilizador pode ser ajustada a partir da organização.',
  'nav.downloads': 'Transferências',
  'nav.aria': 'Navegação do site',
  'nav.ariaMobile': 'Navegação móvel',
  'nav.home': 'Sytue One — Início',
  'nav.openMenu': 'Abrir o menu',
  'nav.closeMenu': 'Fechar o menu',
  'theme.toLight': 'Mudar para o modo claro',
  'theme.toDark': 'Mudar para o modo escuro',
  'lang.label': 'Idioma',
  'cta.start': 'Começar grátis',
  'cta.signIn': 'Iniciar sessão',
  'cta.seeSuite': 'Ver a suite',
  'meta.home.title': 'Sytue One — A suite de produtividade europeia e soberana',
  'meta.home.description':
    'Sytue One: a suite de produtividade europeia e soberana. Identidade, correio, ficheiros, documentos, calendário e chat, 100% na UE e assente em normas abertas.',
  'home.hero.lead': 'A sua produtividade,',
  'home.hero.accent': 'soberana e europeia',
  'home.hero.subtitle':
    'Correio, ficheiros, documentos, calendário, chat e videochamadas — integrados numa única aplicação e alojados inteiramente na UE.',
  'home.hero.note': 'Sem cartão. Dados na Scaleway (UE). Migração assistida.',
  'home.privacy.title': 'O seu trabalho é seu. A sua privacidade também.',
  'home.privacy.body': 'Não vendemos os seus dados nem os usamos para criar perfis publicitários.',
  'home.privacy.anonymity':
    'O Sytue Analytics protege a sua privacidade com estatísticas agregadas, sem cookies de rastreio e com identificadores pseudónimos de visitantes que mudam diariamente e são separados por sítio.',
  'home.privacy.account':
    'Para usar a suite, precisamos de dados da conta e registos de acesso que nos permitam prestar o serviço e mantê-lo seguro.',
  'home.privacy.link': 'Como protegemos a sua privacidade',
  'home.stats.aria': 'Presença europeia',
  'home.stats.regions': 'Regiões da UE (fr-par · nl-ams · zona própria em Madrid)',
  'home.stats.apps': 'Aplicações com uma única identidade',
  'home.stats.standards': 'Normas abertas: OIDC, JMAP, CalDAV, CardDAV e WebDAV',
  'home.products.title': 'Uma suite, uma experiência',
  'home.products.subtitle':
    'Todos os produtos partilham o mesmo design, a mesma identidade e a mesma estrutura. Parecem e funcionam como uma única aplicação.',
  'home.integration.title': 'Integração como uma suite deve funcionar',
  'home.integration.subtitle':
    'Um único registo abre tudo. Aprovisionamento automático (SCIM) a partir do Sytue ID.',
  'home.integration.sso.title': 'Início de sessão único',
  'home.integration.sso.body': 'Uma identidade (Sytue ID) para toda a suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Estrutura comum',
  'home.integration.shell.body':
    'Barra superior, seletor de aplicações, pesquisa e notificações globais em todos os produtos.',
  'home.integration.cross.title': 'Ações entre aplicações',
  'home.integration.cross.body':
    'Anexe a partir do Drive no Mail, transforme um email num evento do Calendar e partilhe-o no Rooms sem interromper o trabalho.',
  'home.sovereignty.title': 'Soberania a sério',
  'home.sovereignty.body':
    'Não é uma promessa de marketing: é a arquitetura. A infraestrutura funciona apenas em regiões europeias, e tudo assenta em protocolos abertos e auditáveis, sem portas traseiras.',
  'home.sovereignty.link': 'Porquê normas abertas',
  'home.sovereignty.eu.title': '100% na UE',
  'home.sovereignty.eu.body':
    'Toda a infraestrutura na UE: Scaleway (fr-par / nl-ams) e uma zona própria em Madrid. Sem fornecedores dos EUA para dados pessoais.',
  'home.sovereignty.gdpr.title': 'Conformidade com o RGPD',
  'home.sovereignty.gdpr.body':
    'Acordo de tratamento de dados, registo das atividades de tratamento, procedimento para pedidos dos titulares e prazos de conservação. Cifrado em trânsito (TLS 1.3) e em repouso.',
  'home.sovereignty.lockin.title': 'Sem dependência',
  'home.sovereignty.lockin.body':
    'Assente em normas abertas: OIDC, JMAP, CalDAV, CardDAV e WebDAV. Os seus dados continuam portáteis.',
  'home.standards.overline': 'Trabalho original da Sytue',
  'home.standards.title': 'Assente em normas abertas',
  'home.standards.body':
    'Cada produto da Sytue é trabalho original, escrito em Go e Astro + Preact sobre protocolos abertos (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Sem motores de terceiros: os seus dados são portáteis e a sua suite é soberana.',
  'home.cta.title': 'Retome o controlo do seu trabalho',
  'home.cta.body':
    'Comece hoje com a sua organização. Migração assistida a partir do Google Workspace ou do Microsoft 365.',
  'home.cta.primary': 'Criar a minha organização',
  'home.cta.secondary': 'Falar com a equipa comercial',
  'grid.discover': 'Descobrir',
  'product.mail.blurb': 'Correio privado e europeu sobre JMAP — sem rastreadores nem publicidade.',
  'product.drive.blurb': 'Ficheiros e colaboração, com a sua própria marca.',
  'product.docs.blurb': 'Documentos, folhas de cálculo e diapositivos com coedição em tempo real.',
  'product.calendar.blurb': 'Um calendário CalDAV ligado ao resto da suite.',
  'product.contacts.blurb': 'Contactos CardDAV, partilhados em toda a organização.',
  'product.notes.blurb':
    'Blocos de notas que guardam tudo: texto, ficheiros, eventos, documentos inteiros.',
  'product.photos.blurb':
    'Galeria e editor de fotografias: os originais no Drive, as edições como receita.',
  'product.rooms.blurb': 'Canais, mensagens e videochamadas de equipa num só lugar.',
  'product.cast.blurb': 'Vídeo a pedido e em direto: carregar, emitir e partilhar, na UE.',
  'product.pages.blurb':
    'Sites e lojas online para a sua empresa, desenhados à sua medida e publicados na UE.',
  'product.bases.blurb':
    'Tabelas relacionais colaborativas para clientes, encomendas ou stocks, na UE.',
  'product.cloud.blurb': 'Uma plataforma de implementação e dados geridos, na UE.',
  'product.id.blurb': 'Uma identidade única (SSO + MFA) com o Sytue ID.',
  'product.one.blurb':
    'A página inicial da suite: aplicações, ações rápidas, atividade recente e avisos.',
  'product.sheets.blurb':
    'Folhas de cálculo com fórmulas, dados externos e gráficos, editadas a várias mãos.',
  'product.slides.blurb': 'Apresentações com a sua marca, temas e vista de apresentador.',
  'product.pdf.blurb': 'Leia, anote, assine e organize os seus PDF sem saírem da Europa.',
  'product.console.blurb':
    'Administração da organização: pessoas, acessos, conformidade e auditoria.',
  'product.erp.blurb':
    'Vinte e quatro módulos e uma contabilidade em partidas dobradas que a base de dados não deixa desequilibrar.',
  'productPage.back': 'A suite completa',
  'productPage.features': 'Tudo o que o {name} faz',
  'productPage.featuresPlanned': 'O que o {name} vai fazer',
  'productPage.builtOn': 'Assente em {standard}',
  'productPage.fromScratch':
    'O editor de Docus, Calcus, Diapos e CertiPDF é escrito de raiz: motor próprio da Sytue, não uma integração de terceiros.',
  'productPage.shot': 'O ecrã principal do {name}, exatamente como aparece ao iniciar sessão.',
  'productPage.shotFeatures': 'O {name} em funcionamento, com as funcionalidades no ecrã.',
  'productPage.open': 'Abrir o {name}',
  'productPage.enDesarrollo': 'Este produto está em desenvolvimento: ainda não está disponível.',
  'productPage.included':
    'Incluído no Sytue One: uma identidade (Sytue ID) e um único registo dão acesso a todos os produtos. Dados 100% na UE (Scaleway) e portáteis através de normas abertas.',
  'product.mail.tagline': 'Correio privado e europeu, sem rastreadores nem publicidade.',
  'product.mail.standard': 'JMAP (norma aberta)',
  'product.mail.p1': 'Uma caixa de entrada rápida com conversas, etiquetas, filtros e pesquisa.',
  'product.mail.p2': 'Anexar ficheiros diretamente a partir do Sytue Drive.',
  'product.mail.p3': 'Transformar um email num evento do Calendar.',
  'product.mail.p4':
    'Cifrado em trânsito (TLS 1.3). A infraestrutura da Sytue está na UE. A localização dos serviços externos depende do respetivo fornecedor.',
  'product.drive.tagline': 'Os seus ficheiros e a sua colaboração, com a marca da sua organização.',
  'product.drive.standard': 'WebDAV (norma aberta)',
  'product.drive.p1':
    'Carregar, organizar e partilhar com ligações controladas (permissões e validade).',
  'product.drive.p2': 'Pré-visualização de documentos e imagens na própria página.',
  'product.drive.p3': 'Anexar no Mail sem duplicar o ficheiro.',
  'product.drive.p4': 'Armazenamento S3 em regiões europeias.',
  'product.docs.tagline':
    'Documentos, folhas de cálculo e diapositivos com coedição em tempo real.',
  'product.docs.standard': 'Coedição em tempo real',
  'product.docs.p1': 'Escrevam em conjunto, com presença em direto e alterações instantâneas.',
  'product.docs.p2': 'Folhas de cálculo com fórmulas e formatação condicional.',
  'product.docs.p3': 'Apresentações com a sua marca.',
  'product.docs.p4': 'Tudo guardado no seu Drive europeu.',
  'product.calendar.tagline': 'O seu calendário, ligado ao resto da suite.',
  'product.calendar.standard': 'CalDAV (norma aberta)',
  'product.calendar.p1': 'Vistas de dia, semana e mês.',
  'product.calendar.p2': 'Eventos periódicos e lembretes.',
  'product.calendar.p3': 'Criar eventos diretamente a partir de um email no Mail.',
  'product.calendar.p4': 'Funciona com qualquer cliente CalDAV.',
  'product.contacts.tagline': 'Os contactos da sua organização, partilhados e portáteis.',
  'product.contacts.standard': 'CardDAV (norma aberta)',
  'product.contacts.p1': 'Um livro de endereços partilhado em toda a organização.',
  'product.contacts.p2': 'Importação e exportação em vCard.',
  'product.contacts.p3': 'Juntar duplicados sem perder dados.',
  'product.contacts.p4': 'Funciona com qualquer cliente CardDAV.',
  'product.notes.tagline': 'Tudo o que quer guardar, num só lugar e nas suas mãos.',
  'product.notes.standard': 'Modelo próprio sobre normas abertas (API REST própria)',
  'product.notes.p1': 'Blocos de notas encaixados, numa tela livre, em colunas ou em lista.',
  'product.notes.p2':
    'Notas de todo o tipo: texto, tarefas, ligações, ficheiros, eventos do Calendar, documentos do Docs.',
  'product.notes.p3': 'Cifra ponto a ponto: a frase-passe nunca sai do seu browser.',
  'product.notes.p4': 'Decide o que entra na pesquisa — e o que nunca entra.',
  'product.rooms.tagline': 'Canais, mensagens e videochamadas de equipa, unificados.',
  'product.rooms.standard': 'Mensagens e chamadas em tempo real',
  'product.rooms.p1': 'Canais por equipa ou projeto.',
  'product.rooms.p2': 'Uma videochamada integrada em cada canal.',
  'product.rooms.p3': 'Menções e notificações unificadas em toda a suite.',
  'product.rooms.p4': 'Histórico pesquisável a partir de qualquer ponto do Sytue One.',
  'product.cloud.p4': 'Domínios próprios com TLS automático.',
  'product.cloud.p3': 'Armazenamento S3 e funções geridos.',
  'product.cloud.p2': 'Base de dados Postgres com API REST automática.',
  'product.cloud.p1': 'Uma pré-visualização para cada PR e reversão imediata.',
  'product.cloud.standard': 'PaaS nativa de Kubernetes',
  'product.cloud.tagline': 'O seu código implementado e os seus dados geridos, na União Europeia.',
  'product.id.tagline': 'Uma identidade para toda a suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (norma aberta)',
  'product.id.p1': 'Início de sessão único em todos os produtos.',
  'product.id.p2': 'MFA e chaves de acesso (WebAuthn).',
  'product.id.p3': 'Aprovisionamento automático (SCIM) a partir do seu diretório.',
  'product.id.p4': 'Perfil e segurança num só lugar.',
  'product.one.tagline': 'Todo o Sytue One à distância de um clique, pela mesma porta.',
  'product.one.standard': 'Um único início de sessão (Sytue ID) para toda a suite',
  'product.one.p1': 'Um lançador com todas as suas aplicações e as que a sua organização liga.',
  'product.one.p2': 'Pesquisa em toda a suite (⌘K) a partir de qualquer ecrã.',
  'product.one.p3':
    'A agenda de hoje e o correio por ler na página inicial, junto ao que é recente na suite e às suas notificações.',
  'product.one.p4': 'Ligue os seus dispositivos e transfira as aplicações nativas.',
  'product.sheets.tagline': 'Números que fazem sentido, numa folha que trabalha consigo.',
  'product.sheets.standard': 'Formatos ODF/OOXML e coedição em tempo real',
  'product.sheets.p1': 'Fórmulas, formatação condicional e validação de dados.',
  'product.sheets.p2': 'Traga dados externos e consulte-os sem sair da folha.',
  'product.sheets.p3': 'Gráficos, gráficos sparkline, segmentações e tabelas de dados.',
  'product.sheets.p4': 'Exportação para ODS e XLSX; abrir ficheiros do Excel.',
  'product.slides.tagline': 'Conte a história; o diapositivo acompanha.',
  'product.slides.standard': 'Formatos ODF/OOXML e coedição em tempo real',
  'product.slides.p1': 'Temas de apresentação com a identidade da sua organização.',
  'product.slides.p2': 'Vista de apresentador com notas e o diapositivo seguinte.',
  'product.slides.p3': 'Imagens, formas, gráficos e SmartArt.',
  'product.slides.p4': 'Exportação para ODP, PPTX e PDF; abrir ficheiros do PowerPoint.',
  'product.pdf.tagline': 'PDF do princípio ao fim, no seu browser.',
  'product.pdf.standard': 'PDF (ISO 32000) com anotações e formulários padrão',
  'product.pdf.p1': 'Realçar, sublinhar, comentar e desenhar sobre o documento.',
  'product.pdf.p2': 'Formulários: preencher campos ou criar os seus.',
  'product.pdf.p3': 'Juntar, extrair, rodar, numerar e marca de água.',
  'product.pdf.p4':
    'Painel de assinaturas, rasura permanente e documentos protegidos por palavra-passe.',
  'product.console.tagline': 'Governe a suite sem sair de um único ecrã.',
  'product.console.standard': 'SCIM, SAML e OpenID Connect (normas abertas)',
  'product.console.p1': 'Pessoas, grupos (também dinâmicos) e funções.',
  'product.console.p2': 'SSO com o seu fornecedor e aprovisionamento SCIM.',
  'product.console.p3':
    'Regras de acesso condicional, dispositivos e acesso privilegiado com aprovação.',
  'product.console.p4': 'Registo de auditoria exportável para o seu SIEM e retenções legais.',
  'product.erp.tagline': 'O ERP completo, com o catálogo de modelos como ficheiro de dados.',
  'product.erp.standard':
    'Contabilidade em partidas dobradas, catálogo de modelos em JSON e uma única API REST',
  'product.erp.p1':
    'Vinte e quatro módulos e trinta e seis modelos: contactos, vendas, compras, inventário, produção, faturação, contabilidade, projetos e recursos humanos.',
  'product.erp.p2':
    'Adicionar um módulo é escrever um ficheiro JSON: sem migração, sem rota nova e sem trabalho de interface.',
  'product.erp.p3':
    'O dinheiro é guardado em cêntimos inteiros, nunca em vírgula flutuante, e é a base de dados que garante o equilíbrio.',
  'product.erp.p4':
    'Os lançamentos registados são intocáveis: corrigem-se com outro lançamento, e é isso que distingue um razão de uma folha de cálculo.',
  'product.one.f1':
    'Um lançador com as aplicações da suite e as de terceiros que a sua organização liga.',
  'product.one.f2':
    'Ações rápidas: escrever um email, novo documento, carregar para o Drive, criar um evento.',
  'product.one.f3': 'Recente em toda a suite: os últimos elementos do Drive, Docus, Mail ou Rooms.',
  'product.one.f4': 'Pesquisa global com ⌘K ou Ctrl K, com resultados de todas as aplicações.',
  'product.one.f5': 'Centro de notificações e presença partilhados em toda a suite.',
  'product.one.f6':
    'Ligar os seus dispositivos: perfil para iPhone e Mac, Android e computador, em dois passos.',
  'product.one.f7': 'Transferências das aplicações nativas para computador e telemóvel.',
  'product.one.f8': 'Um assistente de boas-vindas que prepara a organização sem manuais.',
  'product.mail.f1': 'Conversas agrupadas com leitura, resposta e reencaminhamento com um clique.',
  'product.mail.f2':
    'Pastas, etiquetas coloridas e pesquisa com operadores (from:, has:attachment, is:unread…).',
  'product.mail.f3': 'Regras automáticas que arquivam, etiquetam ou movem o correio à chegada.',
  'product.mail.f4': 'Envio agendado e «Anular envio».',
  'product.mail.f5': 'Assinaturas, aliases de endereço e resposta automática de ausência.',
  'product.mail.f6': 'Caixas de correio partilhadas e delegadas com permissões de leitura e envio.',
  'product.mail.f7': 'Editor de texto formatado com anexos do Sytue Drive sem duplicar ficheiros.',
  'product.mail.f8':
    'Conteúdo remoto bloqueado por predefinição: ninguém fica a saber quando abre um email.',
  'product.mail.f9': 'Correio assinado (PGP e S/MIME) reconhecido e assinalado; TLS em trânsito.',
  'product.mail.f10':
    'Acesso também a partir do Thunderbird, Outlook, Apple Mail ou do telemóvel, com palavras-passe de aplicação.',
  'product.mail.f11':
    'Endereços em etiquetas com o livro de endereços do Contacts à distância de um clique, e um aviso se escrever «em anexo» sem anexar nada.',
  'product.mail.f12':
    'Regras diretamente a partir de um email aberto, esvaziar o lixo de uma vez e atalhos de teclado com «?».',
  'product.drive.f1':
    'A minha unidade, unidades de equipa, partilhados comigo, recentes, favoritos e lixo.',
  'product.drive.f2':
    'Arrastar para carregar; pastas, mover, copiar, mudar o nome e seleção múltipla.',
  'product.drive.f3':
    'Ligações de partilha com dois níveis (ver ou editar), palavra-passe e validade.',
  'product.drive.f4': 'Histórico de versões com restauro.',
  'product.drive.f5': 'Pré-visualizar documentos, imagens e PDF sem transferir.',
  'product.drive.f6':
    'Abrir e editar documentos de escritório no Docus, Calcus e Diapos; PDF no CertiPDF.',
  'product.drive.f7':
    'Etiquetas de sensibilidade que bloqueiam ligações públicas onde não devem existir.',
  'product.drive.f8': 'Partilhar num canal do Rooms ou anexar no Mail sem duplicar o ficheiro.',
  'product.drive.f9':
    'Acesso WebDAV e cliente para computador para ver os seus ficheiros no computador.',
  'product.drive.f10': 'Armazenamento S3 em regiões europeias, com a quota sempre visível.',
  'product.docs.f1': 'Coedição em tempo real, com presença e sem sobreposições.',
  'product.docs.f2': 'Friso com separadores: Base, Inserir, Referências, Correspondência e Rever.',
  'product.docs.f3': 'Tabelas, imagens, formas, SmartArt, gráficos, símbolos e marcas de água.',
  'product.docs.f4': 'Referências: índice, notas de rodapé, citações e bibliografia.',
  'product.docs.f5': 'Correspondência: impressão em série para cartas e etiquetas.',
  'product.docs.f6': 'Comentários em conversa, revisão de alterações e versões.',
  'product.docs.f7':
    'Assistente de IA opcional para reescrever, traduzir ou resumir, só se a sua organização o ativar.',
  'product.docs.f8': 'Exportação para ODT, DOCX e PDF; abrir ficheiros do Word a partir do Drive.',
  'product.docs.f9': 'Cabeçalhos, rodapés, numeração de páginas e blocos reutilizáveis.',
  'product.docs.f10': 'Gravação automática no seu Drive europeu, com histórico.',
  'product.docs.f11':
    'Transferência a partir da página inicial em PDF, HTML, Markdown, OpenDocument ou Microsoft Office.',
  'product.sheets.f1': 'Fórmulas com ajuda contextual e recálculo imediato.',
  'product.sheets.f2':
    'Formatação condicional, validação de dados e formatos numéricos de acordo com a região.',
  'product.sheets.f3': 'Gráficos de vários tipos, sparklines e segmentações.',
  'product.sheets.f4': 'Tabelas de dados com totais e ordenação; tabela dinâmica.',
  'product.sheets.f5': 'Dados externos: ligue uma origem e consulte-a a partir da folha.',
  'product.sheets.f6': 'Solver para encontrar o valor que cumpre um objetivo.',
  'product.sheets.f7': 'Macros para automatizar tarefas repetitivas.',
  'product.sheets.f8': 'Colaboração em tempo real com presença e comentários.',
  'product.sheets.f9': 'Filtros, segmentações e pesquisa dentro da folha.',
  'product.sheets.f10':
    'Exportação para ODS, XLSX e PDF; abrir ficheiros do Excel a partir do Drive.',
  'product.slides.f1': 'Temas e paletas coerentes com a marca da sua organização.',
  'product.slides.f2': 'Tela com formas, imagens, gráficos, SmartArt e conectores.',
  'product.slides.f3': 'Apresentar em ecrã inteiro com vista de apresentador e notas.',
  'product.slides.f4': 'Colaboração em tempo real com presença e comentários.',
  'product.slides.f5': 'Diapositivos com fundos, cartazes e esquemas.',
  'product.slides.f6': 'Exportação para ODP, PPTX e PDF.',
  'product.slides.f7': 'Abrir ficheiros do PowerPoint a partir do Drive.',
  'product.slides.f8': 'Gravação automática com versões.',
  'product.pdf.f1':
    'Visualizador rápido com índice, miniaturas, pesquisa e zoom à página ou à largura.',
  'product.pdf.f2':
    'Anotações: realçar, sublinhar, rasurar, comentar, desenhar, formas, setas e carimbos.',
  'product.pdf.f3': 'Caixas de texto, medições e capturas de áreas.',
  'product.pdf.f4': 'Formulários: preencher campos existentes ou adicionar os seus.',
  'product.pdf.f5': 'Painel de assinatura de documentos.',
  'product.pdf.f6': 'Rasura permanente: o que marcar desaparece do ficheiro, não só da vista.',
  'product.pdf.f7':
    'Páginas: juntar, extrair, eliminar, rodar, numerar (também Bates), cabeçalhos e rodapés.',
  'product.pdf.f8': 'Marca de água, fundo e metadados editáveis.',
  'product.pdf.f9': 'Documentos e permissões protegidos por palavra-passe.',
  'product.pdf.f10': 'Leitura em voz alta e comentários em conversa.',
  'product.calendar.f1': 'Vistas de mês, semana, dia e agenda, com minicalendário e «Hoje».',
  'product.calendar.f2':
    'Repetições diárias, semanais ou mensais com fim; editar só este, este e os seguintes, ou todos.',
  'product.calendar.f3': 'Lembretes por evento e notificações dentro da suite.',
  'product.calendar.f4': 'Convites com resposta (vou, talvez, não) e acompanhamento.',
  'product.calendar.f5': '«Encontrar horário» livre para todas as pessoas convidadas.',
  'product.calendar.f6': 'Salas e recursos reserváveis com disponibilidade.',
  'product.calendar.f7': 'Calendários partilhados com permissões e os que são partilhados consigo.',
  'product.calendar.f8':
    'Subscrever calendários externos (ICS) e manter vários calendários com cores.',
  'product.calendar.f9': 'Criar um evento a partir de uma mensagem do Mail ou de uma nota.',
  'product.calendar.f10':
    'Funciona com qualquer cliente CalDAV: Apple Calendar, Thunderbird e outros.',
  'product.calendar.f11':
    'Duplicar um evento ou copiá-lo para o seu calendário, e transferi-lo em .ics para pessoas fora do Sytue.',
  'product.contacts.f1': 'Livros de endereços, incluindo o partilhado da sua organização.',
  'product.contacts.f2':
    'Ficha completa: telefones, emails, moradas, organização, aniversário, notas e fotografia.',
  'product.contacts.f3': 'Etiquetas e favoritos para agrupar e encontrar.',
  'product.contacts.f4': 'Deteção e junção de duplicados sem perder dados.',
  'product.contacts.f5':
    'Importação e exportação de vCard e CSV; exportação de um contacto individual.',
  'product.contacts.f6': 'Ações em lote e lixo com recuperação.',
  'product.contacts.f7': 'Pesquisa instantânea e preenchimento automático ao escrever no Mail.',
  'product.contacts.f8': 'Funciona com qualquer cliente CardDAV.',
  'product.contacts.f9':
    'Aniversários próximos do seu livro de endereços, com os deste mês assinalados.',
  'product.contacts.f10':
    'Criar uma reunião com um contacto ou copiar o email do contacto sem sair da ficha.',
  'product.notes.f1':
    'Blocos de notas encaixados com ícone e cor; três esquemas: livre, colunas e lista.',
  'product.notes.f2':
    'Doze tipos de nota: texto, tarefas, ligações, cor, imagem e referências ao Drive, Docus, Calendar, Rooms e outras notas.',
  'product.notes.f3': 'Editor com formatação (negrito, listas, ligações) que guarda ao sair.',
  'product.notes.f4': 'Etiquetas, fixar, duplicar, mover entre blocos, agrupar e arrastar na tela.',
  'product.notes.f5':
    'Visibilidade por bloco de notas: privado, pessoal, organização ou ligação pública.',
  'product.notes.f6':
    'Cifra no browser para blocos de notas sensíveis: a Sytue não os consegue ler.',
  'product.notes.f7': 'Decide que blocos de notas entram na pesquisa da suite.',
  'product.notes.f8': 'Retroligações: veja que notas apontam para esta.',
  'product.notes.f9':
    'Exportar um bloco de notas inteiro para HTML, PDF, Markdown e mais; importar cópias.',
  'product.notes.f10': 'Partilhar uma nota num canal do Rooms; largar ou colar para criar notas.',
  'product.notes.f11':
    'Tabelas no editor e atalhos Markdown ao escrever: «- », «# », «> » e «```».',
  'product.notes.f12':
    'Mover uma nota para outro bloco de notas, exportar uma única nota e ordenar o bloco como quiser.',
  'product.photos.tagline': 'As suas fotografias, organizadas e ao seu gosto, sem sair da Europa.',
  'product.photos.standard': 'Originais no Sytue Drive; motor de edição no seu browser',
  'product.photos.p1': 'Cronologia por mês com pesquisa por nome.',
  'product.photos.p2': 'Álbuns com capa; visualizador em ecrã inteiro.',
  'product.photos.p3': 'Editor de luz, cor, rotação, espelho e recorte, em tempo real.',
  'product.photos.p4': 'Cada edição é uma receita: o original nunca é alterado.',
  'product.photos.f1': 'Cronologia agrupada por mês, com pesquisa por nome e miniaturas nítidas.',
  'product.photos.f2':
    'Álbuns: criar, mudar o nome, escolher capa, adicionar a partir do visualizador e remover.',
  'product.photos.f3': 'Visualizador em ecrã inteiro com navegação pelo teclado.',
  'product.photos.f4':
    'Editor com quatro controlos deslizantes (exposição, contraste, saturação, temperatura), rotação e espelho.',
  'product.photos.f5':
    'Recorte por arrastamento e atalhos de teclado; pré-visualização na GPU com a CPU como alternativa.',
  'product.photos.f6':
    'Cada edição é uma receita guardada à parte: o original no Drive nunca é modificado.',
  'product.photos.f7':
    'Exportação em resolução total, processada no servidor com as mesmas fórmulas da pré-visualização.',
  'product.photos.f8': 'Lixo com restauro.',
  'product.photos.f9':
    'Os originais ficam no seu Sytue Drive; os derivados são uma cache regenerável.',
  'product.photos.f10': 'Dados EXIF lidos do próprio ficheiro; tudo na União Europeia.',
  'product.photos.f11':
    'Seleção múltipla por mês: para um álbum novo, para o lixo ou transferir de uma vez.',
  'product.photos.f12':
    'Apresentação de diapositivos, tamanho real, rotação a partir do visualizador e recorte com proporção fixa.',
  'product.rooms.f1': 'Canais públicos e privados por categorias; mensagens diretas e grupos.',
  'product.rooms.f2': 'Conversas, reações, menções, mensagens afixadas e guardadas, e rascunhos.',
  'product.rooms.f3':
    'Formatação com a barra «Aa» e atalhos: negrito, itálico, código, citações, listas e ligações.',
  'product.rooms.f4': 'Anexos, imagens, GIF e emojis personalizados; ficheiros do Drive.',
  'product.rooms.f5': 'Envio agendado, prioridade, confirmação de leitura e «visto por».',
  'product.rooms.f6': 'Comandos com barra e cartões: sondagens e tarefas atribuíveis.',
  'product.rooms.f7':
    'Videochamadas no próprio canal: câmara, ecrã, mão levantada, chat e gravação.',
  'product.rooms.f8': 'Reuniões agendadas e ligações de convite.',
  'product.rooms.f9': 'Bots, webhooks e integrações com outras ferramentas.',
  'product.rooms.f10': 'Presença e estado personalizado; definições de notificação por canal.',
  'product.rooms.f11': 'Pesquisa no histórico, também a partir de toda a suite.',
  'product.rooms.f12': 'Tempo real sobre WebSocket, com religação e sem mensagens perdidas.',
  'product.rooms.f13':
    'Mudança rápida de canal com Ctrl+G, marcar tudo como lido e atalhos com «?».',
  'product.cast.tagline':
    'Os seus vídeos e os seus diretos, a pedido e em tempo real, sem sair da Europa.',
  'product.cast.standard':
    'Reprodução HLS no navegador; ingestão RTMP a partir de qualquer programa de emissão',
  'product.cast.p1':
    'Carregue um vídeo e é transcodificado em várias qualidades para qualquer ecrã.',
  'product.cast.p2':
    'Entre em direto a partir de um programa de emissão, do navegador ou da aplicação.',
  'product.cast.p3':
    'Quatro níveis de visibilidade: público, não listado, organização ou pessoas específicas.',
  'product.cast.p4': 'Chat em direto e comentários, com moderação básica por quem publica.',
  'product.cast.f1': 'Carregue vídeos até 8 GiB; o original é conservado.',
  'product.cast.f2': 'Reprodução adaptativa em três qualidades; o navegador escolhe a melhor.',
  'product.cast.f3':
    'Direto a partir de um programa de emissão por RTMP, com uma chave de emissão por pessoa.',
  'product.cast.f4': 'Direto a partir do navegador ou da aplicação: câmara, ecrã e microfone.',
  'product.cast.f5': 'As gravações dos diretos tornam-se vídeos a pedido automaticamente.',
  'product.cast.f6': 'Uma página inicial pública e um canal por organização, indexáveis.',
  'product.cast.f7':
    'Ligações não listadas, vídeos só para a organização ou acesso a pessoas específicas por @utilizador.',
  'product.cast.f8':
    'Chat em direto e comentários em fio, moderados por quem publica e pela administração da organização.',
  'product.cast.f9': 'Leitor incorporável e oEmbed para os vídeos públicos e não listados.',
  'product.cast.f10': 'O armazenamento conta na sua quota do Sytue Drive; tudo na União Europeia.',
  'product.cast.f11':
    '«Carregar no Cast» a partir do Sytue Drive: uma cópia do vídeo passa para o Cast e o ficheiro fica no Drive.',
  'product.pages.tagline':
    'O site da sua empresa e a respetiva loja, desenhados num editor visual e publicados sem sair da Europa.',
  'product.pages.standard':
    'HTML e CSS estáticos e acessíveis, sem cookies de terceiros; exportação para Astro',
  'product.pages.p1':
    'Um editor visual com molduras que se adaptam sozinhas ao computador, ao tablet e ao telemóvel.',
  'product.pages.p2':
    'Dez modelos por setor, com textos verdadeiros e fotografias de domínio público.',
  'product.pages.p3': 'Uma loja com catálogo, carrinho e cobrança na conta da sua empresa.',
  'product.pages.p4':
    'O seu próprio domínio com certificado automático, ou um endereço em sites.sytue.eu.',
  'product.pages.f1':
    'Editor visual por molduras com disposição automática, componentes reutilizáveis e secções prontas.',
  'product.pages.f2':
    'Três pontos de quebra (computador, tablet e telemóvel), cada um com os seus próprios ajustes.',
  'product.pages.f3': 'Análise de visitas sem cookies, dentro do painel do site.',
  'product.pages.f4':
    'Sites em vários idiomas, com endereços por idioma, hreflang e tradução assistida pelo Sytue AI.',
  'product.pages.f5':
    'Imagens do seu computador, do Sytue Drive ou do Sytue Photos, com texto alternativo pedido ao inseri-las.',
  'product.pages.f6':
    'Loja com variantes, stock, IVA por produto e pagamento com Mollie, Adyen, Redsys ou Stripe.',
  'product.pages.f7':
    'Encomendas com fatura simplificada emitida pelo Sytue ERP e livre resolução a partir do próprio site.',
  'product.pages.f8':
    'Formulários com consentimento, caixa de mensagens e avisos às pessoas da sua organização.',
  'product.pages.f9':
    'Versões com nome, pré-visualização atrás de uma ligação secreta e regresso a qualquer publicação anterior.',
  'product.pages.f10':
    'Exportação do código para Astro, React, Next.js e aplicações nativas, e do catálogo para Shopify, WooCommerce e outras plataformas.',
  'product.bases.tagline':
    'Tabelas ligadas para gerir clientes, encomendas ou stocks, com permissões e registo de cada alteração.',
  'product.bases.standard':
    'CSV e JSON abertos; esquema e relações com integridade garantida pela própria base de dados',
  'product.bases.p1':
    'Clientes, encomendas ou stocks em tabelas ligadas, sem saber nada de bases de dados.',
  'product.bases.p2': 'Partilhe com a sua organização com a função que decidir.',
  'product.bases.p3': 'Cada alteração fica registada: quem a fez e qual era o valor anterior.',
  'product.bases.p4': 'Os seus dados ficam na UE e exporta-os quando quiser.',
  'product.bases.f1':
    'Tabelas com campos de texto, número, data, caixa de verificação, escolha única e escolha múltipla.',
  'product.bases.f2':
    'Relações entre tabelas: ligue uma encomenda ao seu cliente e a base de dados não deixa quebrar a ligação.',
  'product.bases.f3':
    'Grelha editável com o teclado, legível por leitores de ecrã e utilizável no telemóvel.',
  'product.bases.f4':
    'Permissões por base e por tabela: proprietário, editor, comentador e leitor.',
  'product.bases.f5': 'Importe um CSV como tabela nova e exporte para CSV ou JSON.',
  'product.bases.f6': 'Registo de cada alteração: quem, que registo e campo, antes e depois.',
  'product.cloud.f12': 'Consumo, custos e faturação europeia na mesma consola.',
  'product.cloud.f11': 'Pessoas, tokens de API e registo de auditoria exportável.',
  'product.cloud.f10': 'Análise web que respeita a privacidade, sem cookies.',
  'product.cloud.f9': 'Tarefas agendadas e modelos de projeto.',
  'product.cloud.f8': 'Terminal dentro do contentor; reiniciar ou pausar o serviço.',
  'product.cloud.f7': 'Registos em direto, métricas e monitorização com alertas.',
  'product.cloud.f6': 'Variáveis de ambiente e segredos por ambiente.',
  'product.cloud.f5': 'Domínios próprios com TLS automático; redirecionamentos e cabeçalhos.',
  'product.cloud.f4': 'Armazenamento S3 e funções geridas.',
  'product.cloud.f3': 'Bases de dados Postgres com API REST automática e editor SQL.',
  'product.cloud.f2':
    'Uma pré-visualização para cada PR e reversão para qualquer implementação com um clique.',
  'product.cloud.f1':
    'Projetos a partir do seu repositório (GitHub, GitLab ou o seu próprio Git) com implementação a cada push.',
  'product.console.f1': 'Painel com o estado da organização e o que merece atenção hoje.',
  'product.console.f2':
    'Contas: entrada, suspensão, reposição de palavra-passe e MFA, sessões, aliases e caixa de correio.',
  'product.console.f3': 'Grupos estáticos e grupos dinâmicos por regras; funções com permissões.',
  'product.console.f4': 'Acesso privilegiado (PIM): ativação temporária com aprovação.',
  'product.console.f5':
    'SSO com fornecedores SAML ou OIDC, com teste de ligação; aprovisionamento SCIM.',
  'product.console.f6': 'Regras de acesso condicional e inventário de dispositivos de confiança.',
  'product.console.f7':
    'Integrações: clientes OIDC, SAML e ligações a terceiros; substituição de produtos.',
  'product.console.f8': 'Trazer conteúdo: migração a partir de outros fornecedores.',
  'product.console.f9': 'Editor de documentos e canais do Rooms administráveis.',
  'product.console.f10':
    'Governação: retenções legais, etiquetas de sensibilidade e política de palavras-passe.',
  'product.console.f11':
    'Registo de auditoria com exportação CSV e reencaminhamento para o seu SIEM; registos dos serviços.',
  'product.console.f12': 'Domínio e marca próprios: logótipo da organização e domínio de correio.',
  'product.id.f1':
    'Início de sessão único (OpenID Connect e OAuth 2.0 com PKCE) para a suite e as suas aplicações.',
  'product.id.f2': 'Chaves de acesso (WebAuthn) e MFA TOTP com códigos de recuperação.',
  'product.id.f3': 'Palavras-passe de aplicação para IMAP e clientes que não suportam OIDC.',
  'product.id.f4': 'Sessões e dispositivos: revogar uma ou todas as outras.',
  'product.id.f5': 'Federação SAML com o seu IdP, e o Sytue como IdP SAML para outras aplicações.',
  'product.id.f6': 'Aprovisionamento SCIM a partir do seu diretório.',
  'product.id.f7': 'Convites, verificação de email e recuperação de palavra-passe.',
  'product.id.f8': 'Perfil para iPhone e Mac (.mobileconfig) com um toque.',
  'product.id.f9': 'Políticas de palavra-passe e regras de acesso definidas pela organização.',
  'product.id.f10': 'Registo dinâmico de clientes (DCR) e descoberta padrão.',
  'product.erp.f1':
    'Vinte e quatro módulos e trinta e seis modelos num único serviço e numa única base de dados.',
  'product.erp.f2':
    'O catálogo de modelos são dados: cada módulo declara os seus campos, os seus estados e as colunas da sua lista.',
  'product.erp.f3':
    'Um módulo novo é um ficheiro JSON — sem DDL, sem migração e sem bloqueio de tabelas.',
  'product.erp.f4':
    'Uma API para os trinta e seis modelos e uma interface gerada a partir do catálogo.',
  'product.erp.f5':
    'Contabilidade em partidas dobradas com montantes em cêntimos inteiros; nem um número em vírgula flutuante.',
  'product.erp.f6':
    'É a base de dados que verifica o equilíbrio ao confirmar a transação, não o código da aplicação.',
  'product.erp.f7':
    'Os lançamentos fechados são imutáveis: não se podem editar, apagar nem receber linhas novas.',
  'product.erp.f8':
    'Um documento, um lançamento: uma nova tentativa ou dois separadores abertos não podem duplicar receitas.',
  'product.erp.f9':
    'Os rótulos comuns existem nos 26 idiomas, e um campo sem tradução impede o arranque do serviço.',
  'product.erp.f10':
    'Listagens com índices: a primeira página de vinte mil linhas chega em menos de um milissegundo.',
  'meta.downloads.title': 'Transferências — Sytue One para computador e telemóvel',
  'meta.downloads.description':
    'Aplicações nativas do Sytue One para macOS, Windows, Linux, iOS e Android. A suite completa no seu dispositivo, com as integrações de cada sistema.',
  'downloads.overline': 'Aplicações',
  'downloads.h1': 'O Sytue One no seu dispositivo',
  'downloads.intro':
    'Aplicações de computador e telemóvel a sério: instalam-se, ficam na Dock ou no menu Iniciar, avisam através das notificações do sistema e abrem ficheiros do seu dispositivo.',
  'downloads.desktop.title': 'Computador',
  'downloads.desktop.body':
    'Uma aplicação nativa por produto, que desenha a sua própria interface com a placa gráfica: sem navegador lá dentro, arranca depressa e consome como uma aplicação do sistema.',
  'downloads.apps.title': 'Só o que precisa',
  'downloads.apps.body':
    'O Sytue One cobre o dia de trabalho inteiro. Se só precisar da unidade, o Sytue Drive instala-se em separado.',
  'downloads.includes': 'Inclui:',
  'downloads.mobile.title': 'Telemóvel',
  'downloads.mobile.body':
    'Aplicações nativas para iOS e Android, com notificações do sistema, partilha e acesso a ficheiros e câmara.',
  'downloads.selfhost.title': 'A sua organização tem domínio próprio?',
  'downloads.selfhost.body':
    'As aplicações nativas ligam-se a sytue.eu. Se a sua organização tiver uma instalação própria, entre com o navegador no domínio dela: a suite funciona da mesma forma na web.',
  'downloads.yourSystem': 'O seu sistema',
  'downloads.signInToGet': 'Inicie sessão para transferir',
  'downloads.store': 'Ver na {format}',
  'downloads.soon': 'Em breve na {format}',
  'downloads.browser.text':
    'Prefere o browser? A suite completa funciona igual na web, sem instalar nada:',
  'downloads.browser.link': 'abrir o Sytue One',
  'platform.macos.req': 'macOS 11 ou posterior · Apple Silicon e Intel',
  'platform.windows.req': 'Windows 10 ou posterior · 64 bits',
  'platform.linux.req': 'OpenGL 2.1 · 64 bits',
  'platform.ios.req': 'iOS 14 ou posterior',
  'platform.android.req': 'Android 6.0 ou posterior',
  'footer.tagline':
    'Cada aplicação da suite é um serviço próprio, construído sobre protocolos abertos e publicados.',
  'footer.products': 'Produtos',
  'footer.resources': 'Recursos',
  'footer.legal': 'Legal',
  'footer.suite': 'A suite',
  'footer.downloads': 'Transferir as aplicações',
  'footer.sales': 'Falar com a equipa comercial',
  'footer.privacy': 'Privacidade e RGPD',
  'footer.accessibility': 'Acessibilidade',
  'footer.openSource': 'Código aberto',
  'footer.legalNotice': 'Aviso legal',
  'footer.terms': 'Termos',
  'footer.copyright':
    '© {year} Sytue · Dados alojados na UE (Scaleway fr-par / nl-ams · zona própria em Madrid).',
  'footer.privacyNotice':
    'Não vendemos os seus dados nem os partilhamos com terceiros. Não os usamos para criar perfis, para publicidade nem para treinar ou melhorar os nossos serviços ou os de terceiros. São cifrados em trânsito e em repouso, e só saem da nossa infraestrutura europeia quando é estritamente necessário para faturar, para enviar notificações com a aplicação fechada (o serviço de push só vê o seu dispositivo e a hora, nunca o conteúdo) e nas integrações que ativar, em que o terceiro pedirá a sua autorização para os dados que vai usar.',
  'meta.notFound.title': 'Página não encontrada — Sytue One',
  'notFound.overline': 'Erro 404',
  'notFound.title': 'Não encontrámos esta página',
  'notFound.body': 'A ligação pode ter mudado. Volte ao início ou explore a suite.',
  'notFound.home': 'Voltar ao início',
  'meta.legal.title': 'Aviso legal — Sytue One',
  'meta.legal.description':
    'Dados de identificação do titular de www.sytue.eu e do serviço Sytue One: Sytue Projects, S.L.',
  'aviso.title': 'Aviso legal',
  'aviso.intro':
    'Em cumprimento do artigo 10.º da Lei espanhola 34/2002, de 11 de julho, relativa aos serviços da sociedade da informação e ao comércio eletrónico (LSSI), apresentam-se os dados de identificação do titular deste sítio web e do serviço Sytue One.',
  'aviso.review.body':
    'Os dados registais deste aviso foram verificados no Registo Comercial. O resto do texto está em revisão jurídica e a sua redação pode mudar sem que mude o que descreve.',
  'aviso.owner.title': 'Titular',
  'aviso.owner.name': 'Denominação social',
  'aviso.owner.form': 'Forma jurídica',
  'aviso.owner.formValue':
    'Sociedade unipessoal por quotas (sociedad limitada unipersonal) de direito espanhol',
  'aviso.owner.nif': 'Número de identificação fiscal (NIF)',
  'aviso.owner.address': 'Sede social',
  'aviso.owner.registry': 'Registo',
  'aviso.owner.registryValue':
    'Registo Comercial de Madrid (Registro Mercantil de Madrid), secção 8, folha M-887153, inscrição 1',
  'aviso.owner.email': 'Email',
  'aviso.owner.web': 'Sítio web',
  'aviso.scope.title': 'Objeto',
  'aviso.scope.body':
    'Este sítio web apresenta a suite Sytue One e dá acesso às suas aplicações. A utilização do serviço rege-se pelos Termos de Serviço, e o tratamento de dados pessoais pela Política de Privacidade, ambos com ligação no rodapé de cada página.',
  'aviso.ip.title': 'Propriedade intelectual e industrial',
  'aviso.ip.body':
    'Os conteúdos deste sítio (textos, design, código e as marcas e logótipos Sytue e Sytue One) pertencem à Sytue Projects, S.L. ou são utilizados sob licença. Os nomes de produtos de terceiros apresentados como referência de compatibilidade são marcas dos respetivos titulares.',
  'aviso.liability.title': 'Responsabilidade',
  'aviso.liability.body':
    'A Sytue Projects, S.L. esforça-se por manter este sítio disponível e atualizado, sem garantir a ausência de interrupções ou erros. Não é responsável pelo conteúdo de sítios de terceiros para os quais existam ligações.',
  'aviso.law.title': 'Lei aplicável',
  'aviso.law.body':
    'Este aviso rege-se pela lei espanhola. Em relações de consumo, mantêm-se os direitos e o foro conferidos pela lei do país de residência na União Europeia.',
  'aviso.updated': 'Última atualização: setembro de 2026.',
  'meta.accessibility.title': 'Declaração de acessibilidade — Sytue One',
  'meta.accessibility.description':
    'Quão acessíveis são a suite Sytue One e este sítio web, o que medimos, o que ainda falta e como nos pode avisar.',
  'a11y.title': 'Declaração de acessibilidade',
  'a11y.intro':
    'A Sytue quer que qualquer pessoa possa utilizar o Sytue One e este sítio web, qualquer que seja a tecnologia de apoio que utilize. Esta declaração abrange www.sytue.eu e as aplicações web da suite.',
  'a11y.standard.title': 'A norma que seguimos',
  'a11y.standard.body':
    'Concebemos e desenvolvemos de acordo com as WCAG 2.1 nível AA, o nível referido pela norma europeia EN 301 549 e pelo Ato Europeu da Acessibilidade (Diretiva (UE) 2019/882).',
  'a11y.status.title': 'Estado de conformidade',
  'a11y.status.body':
    'Parcialmente conforme: a maior parte da suite cumpre a norma e as partes indicadas abaixo ainda não a cumprem totalmente. Ainda não foi encomendada uma auditoria independente.',
  'a11y.measures.title': 'O que fazemos para lá chegar',
  'a11y.measures.design':
    'Um único sistema de design para toda a suite: cores de texto com contraste verificado, indicador de foco visível em cada controlo e utilização de menus, caixas de diálogo e listas por teclado.',
  'a11y.measures.ci':
    'Auditorias de acessibilidade automáticas (Lighthouse) correm na pipeline de integração em cada alteração do sítio web público.',
  'a11y.measures.system':
    'A interface respeita as definições do seu sistema: tema escuro ou claro, movimento reduzido e tamanho do texto, sem tamanhos fixos em píxeis nos campos de formulário.',
  'a11y.measures.text':
    'Cada imagem, ícone e estado vazio tem alternativa em texto, e cada página declara o seu idioma para que os leitores de ecrã a pronunciem corretamente.',
  'a11y.limits.title': 'Limitações conhecidas',
  'a11y.limits.body':
    'A grelha das folhas de cálculo, a tela dos diapositivos e a anotação de PDF são editores complexos que ainda precisam de trabalho para serem totalmente utilizáveis com teclado e leitor de ecrã. Os documentos e ficheiros carregados na plataforma são mostrados tal como estão e não os adaptamos.',
  'a11y.feedback.title': 'Diga-nos o que não funciona',
  'a11y.feedback.body':
    'Se algo na suite ou neste sítio web for difícil ou impossível de utilizar, escreva-nos e vamos analisar e responder:',
  'a11y.enforcement.title': 'Se a resposta não for suficiente',
  'a11y.enforcement.body':
    'Se a nossa resposta a uma barreira comunicada não for suficiente, pode dirigir-se à autoridade responsável pela acessibilidade no seu país ao abrigo do Ato Europeu da Acessibilidade.',
  'a11y.updated':
    'Esta declaração foi redigida em setembro de 2026 e é revista quando a suite muda de forma significativa.',
  'meta.openSource.title': 'Código aberto — Sytue One',
  'meta.openSource.description':
    'A Sytue constrói quase tudo de raiz, mas não reinventa o que já funciona bem. Os projetos de código aberto em que a suite se apoia.',
  'oss.title': 'Código aberto',
  'oss.intro':
    'A Sytue constrói quase tudo de raiz: o correio, os editores, o calendário. Mas não reinventa o que já funciona bem. Estes são os projetos de código aberto em que se apoiam algumas partes da suite.',
  'oss.livekit.body':
    'O áudio e vídeo em tempo real das chamadas do Sytue Rooms corre sobre o LiveKit.',
  'oss.postgres.body':
    'Cada serviço da suite guarda os seus dados em PostgreSQL, uma base de dados por organização.',
  'meta.privacy.title': 'Privacidade e RGPD — Sytue One',
  'meta.privacy.description':
    'Como o Sytue One trata dados pessoais: responsável pelo tratamento, dados, fundamentos jurídicos, conservação, subcontratantes, segurança, transferências internacionais e os seus direitos ao abrigo do RGPD.',
  'legal.overline': 'Legal',
  'legal.updated': 'Última atualização: julho de 2026.',
  'legal.review.title': 'Em revisão jurídica.',
  'privacy.title': 'Privacidade e RGPD',
  'privacy.review.body':
    'Esta política descreve exatamente como tratamos os dados hoje. Os prazos concretos de conservação serão completados na versão final, revista pela nossa equipa jurídica.',
  'privacy.controller.title': 'Responsável pelo tratamento',
  'privacy.controller.body':
    'A Sytue Projects, S.L. (NIF espanhol B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Espanha; inscrita no Registo Comercial de Madrid, secção 8, folha M-887153), enquanto fornecedora da suite Sytue One, é a responsável pelo tratamento dos dados das contas e organizações do serviço. Contacto para proteção de dados:',
  'privacy.data.title': 'Que dados tratamos',
  'privacy.data.account.label': 'Dados da conta:',
  'privacy.data.account.body':
    'nome, endereço de email e organização, geridos pelo Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Conteúdo da sua organização:',
  'privacy.data.content.body':
    'correio, ficheiros, documentos, calendário e mensagens que a sua organização aloja na suite. Esse conteúdo pertence à sua organização.',
  'privacy.data.technical.label': 'Dados técnicos de funcionamento:',
  'privacy.data.technical.body':
    'registos de acesso e dados de ligação necessários para a segurança, a prevenção de abusos e o funcionamento do serviço.',
  'privacy.data.note':
    'Não vendemos dados nem os usamos para publicidade. A infraestrutura e o armazenamento geridos pela Sytue encontram-se na União Europeia (Scaleway, regiões fr-par e nl-ams, e uma zona local em Madrid), com encriptação em trânsito (TLS 1.3) e em repouso. As ligações a serviços externos enviam os dados que autorizas para o respetivo fornecedor, de acordo com a sua localização e condições.',
  'privacy.basis.title': 'Fundamento jurídico',
  'privacy.basis.body':
    'O tratamento baseia-se nos fundamentos jurídicos do artigo 6.º do RGPD: execução do contrato de serviço com a sua organização, o nosso interesse legítimo na segurança e na prevenção de abusos, o cumprimento de obrigações legais e, quando expressamente solicitado, o seu consentimento. O detalhe fundamento a fundamento, tratamento a tratamento, fará parte do texto final.',
  'privacy.retention.title': 'Conservação',
  'privacy.retention.body':
    'Conservamos os dados da conta e o conteúdo enquanto a sua organização mantiver o serviço ativo. Ao encerrar uma conta ou organização, os dados são apagados ou anonimizados num prazo razoável, exceto o que tenhamos de conservar por lei (a faturação, por exemplo). Os registos técnicos de funcionamento são purgados periodicamente. Os prazos concretos por categoria são fixados na versão final. As denúncias de conteúdo são conservadas enquanto estão abertas e apagadas doze meses depois de resolvidas.',
  'privacy.processors.title': 'Subcontratantes e subcontratantes ulteriores',
  'privacy.processors.body':
    'O nosso fornecedor de infraestrutura é a Scaleway (regiões fr-par e nl-ams), que atua como subcontratante e opera exclusivamente na União Europeia; além disso, operamos uma zona própria em Madrid (Espanha) em instalações próprias. Para os pagamentos das subscrições recorremos à Mollie B.V. (Países Baixos), que trata apenas os dados necessários para o pagamento. Não recorremos a subcontratantes ulteriores fora do EEE. A lista de subcontratantes ulteriores será publicada e mantida atualizada, com aviso prévio de qualquer inclusão.',
  'privacy.security.title': 'Segurança',
  'privacy.security.body':
    'Aplicamos medidas técnicas e organizativas de acordo com o RGPD: cifra em trânsito (TLS 1.3) e em repouso, isolamento entre organizações (multi-tenant com segurança ao nível da linha), autenticação forte (MFA/WebAuthn), registo de acessos e privilégio mínimo. Nenhum sistema é infalível: se uma violação afetar os seus dados, enviaremos uma notificação nos prazos previstos pelo Regulamento.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'Usamos apenas cookies técnicos necessários ao início de sessão único e à segurança da sessão. Não usamos cookies publicitários nem de rastreio de terceiros, pelo que não há barreira de consentimento para rastreio.',
  'privacy.transfers.title': 'Transferências internacionais',
  'privacy.transfers.body':
    'O tratamento e o alojamento geridos pela Sytue ocorrem em regiões da União Europeia. As ligações externas ativadas pelo utilizador enviam os dados autorizados para o respetivo fornecedor; a sua localização depende do fornecedor e da sua configuração. As notificações enviadas com a aplicação ou o browser fechados também podem usar serviços externos. O serviço de push do dispositivo (Google, Mozilla ou Apple, conforme o seu browser ou telemóvel) vê um identificador anónimo do seu dispositivo e a hora de cada notificação — nunca o conteúdo, que viaja cifrado ponto a ponto e, no iOS, é enviado vazio. Pode desativar as notificações, e então nem isso sai.',
  'privacy.rights.title': 'Os seus direitos',
  'privacy.rights.body1':
    'O RGPD reconhece-lhe os direitos de acesso, retificação, apagamento, oposição, limitação do tratamento e portabilidade. Pode exercê-los escrevendo para',
  'privacy.rights.body2':
    '; respondemos nos prazos previstos pelo Regulamento. Tem também o direito de apresentar reclamação junto',
  'privacy.rights.authority': 'da sua autoridade de controlo',
  'privacy.dpo.title': 'Encarregado da proteção de dados (DPO)',
  'privacy.dpo.body':
    'Para qualquer questão sobre proteção de dados, ou para contactar a equipa de proteção de dados (DPO):',
  'privacy.minors.title': 'Menores',
  'privacy.minors.body':
    'O Sytue One é um serviço para organizações e utilização profissional; não se destina a menores e não recolhemos conscientemente dados de menores.',
  'privacy.changes.title': 'Alterações a esta política',
  'privacy.changes.body':
    'Se atualizarmos esta política, mudaremos a data de «última atualização» e, quando as alterações forem relevantes, avisaremos pelos canais do serviço antes de entrarem em vigor.',
  'meta.terms.title': 'Termos de serviço — Sytue One',
  'meta.terms.description':
    'Termos de serviço do Sytue One: fornecedor, âmbito, contas e organizações, utilização aceitável, disponibilidade, os seus dados, propriedade intelectual e lei aplicável.',
  'terms.title': 'Termos de serviço',
  'terms.review.body':
    'Estes termos descrevem o serviço e a sua utilização exatamente como funciona hoje. As cláusulas contratuais concretas (nível de serviço, responsabilidade e foro) serão completadas na versão final, revista pela nossa equipa jurídica.',
  'terms.provider.title': 'Quem presta o serviço',
  'terms.provider.body':
    'O Sytue One é prestado pela Sytue Projects, S.L. (NIF espanhol B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Espanha; inscrita no Registo Comercial de Madrid, secção 8, folha M-887153) a partir de infraestrutura localizada exclusivamente na União Europeia (Scaleway, regiões fr-par e nl-ams, e uma zona própria em Madrid), e rege-se pelo direito da UE.',
  'terms.scope.title': 'Âmbito do serviço',
  'terms.scope.body':
    'O Sytue One é uma suite de produtividade para organizações: identidade (Sytue ID), correio, ficheiros, documentos, calendário, contactos e mensagens com videochamadas. O serviço é contratado por organização.',
  'terms.accounts.title': 'Contas e organizações',
  'terms.accounts.body':
    'Cada organização gere as suas contas, grupos e permissões através do Sytue ID (SSO + MFA). A administração da organização é responsável por adicionar e remover contas e pela utilização do serviço através delas.',
  'terms.use.title': 'Utilização aceitável',
  'terms.use.body':
    'Compromete-se a utilizar o serviço de forma lícita e a não o usar para atividades ilícitas, spam, distribuição de malware, violação de direitos de terceiros ou qualquer utilização que comprometa a segurança ou a disponibilidade do serviço para outras organizações.',
  'terms.availability.title': 'Disponibilidade e suporte',
  'terms.availability.body1':
    'Trabalhamos para oferecer elevada disponibilidade e fazemos manutenção e melhorias de forma contínua. Os compromissos concretos de nível de serviço (SLA), quando aplicáveis ao seu plano, constam do contrato de serviço. O suporte é prestado através de',
  'terms.data.title': 'Os seus dados',
  'terms.data.body1':
    'O conteúdo que a sua organização aloja na suite pertence à sua organização. A suite assenta em normas abertas (OIDC, JMAP, CalDAV, CardDAV, WebDAV), pelo que os seus dados continuam portáteis e pode levá-los consigo. O tratamento de dados pessoais é descrito na página',
  'terms.ip.title': 'Propriedade intelectual',
  'terms.ip.body':
    'O software Sytue One é obra original da Sytue e está protegido pelos direitos de propriedade intelectual correspondentes. Mantém a titularidade do conteúdo que a sua organização aloja na suite; só o tratamos para prestar o serviço.',
  'terms.liability.title': 'Responsabilidade',
  'terms.liability.body':
    'Esforçamo-nos ao máximo por um serviço fiável e seguro. As garantias e limitações de responsabilidade aplicáveis constam do contrato de serviço; nada nestes termos exclui responsabilidades que a lei não permita excluir.',
  'terms.contact.title': 'Contratação e contacto',
  'terms.contact.body1':
    'Para condições de contratação, preços por organização ou qualquer questão sobre o serviço, escreva para',
  'terms.changes.title': 'Alterações',
  'terms.changes.body':
    'Podemos atualizar estes termos para refletir alterações no serviço ou na legislação. Quando as alterações forem relevantes, avisaremos pelos canais do serviço antes de entrarem em vigor, atualizando a data de «última atualização».',
  'terms.law.title': 'Lei aplicável',
  'terms.law.body':
    'O serviço rege-se pelo direito da União Europeia e pela lei do país do fornecedor. O foro concreto para a resolução de litígios é fixado no texto final. Independentemente disso, mantém o direito de apresentar reclamação junto da autoridade de controlo em matéria de proteção de dados.',
  'seo.org.description':
    'Sytue One: a suite de produtividade europeia e soberana. Identidade, correio, ficheiros, documentos, calendário, chat, videochamadas e PaaS, 100% na UE.',
  'seo.org.slogan': 'A sua produtividade, soberana e europeia.',
  'seo.software.description':
    'Suite de produtividade europeia e soberana: correio (JMAP), ficheiros (WebDAV), documentos colaborativos, calendário (CalDAV), contactos (CardDAV), chat, videochamadas e PaaS. SSO com o Sytue ID.',
  'seo.software.f1': 'Início de sessão único (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Correio, ficheiros, documentos, calendário, contactos, chat e videochamadas',
  'seo.software.f3': 'Plataforma de implementação e dados geridos',
  'seo.software.f4': 'Dados 100% na UE (Scaleway)',
  'seo.offer.description': 'Período de teste gratuito, sem cartão.',
  'seo.downloads.description':
    'Aplicações nativas da suite Sytue One para computador (macOS, Windows, Linux) e telemóvel (iOS, Android).',
  'home.pricing.personal.title': 'Grátis',
  'home.pricing.personal.freeStorage': '5 GB, para uma pessoa',
  'home.pricing.personal.freeBody':
    'Correio, ficheiros, fotografias, documentos, calendário, contactos, notas e salas. Sem cartão e sem limite de tempo.',
  'home.pricing.personal.note':
    'A sua organização é o seu email: sem administração de organização e sem ERP, e tudo o resto, incluindo o Sytue Admin para as suas integrações e migrações.',
  'home.pricing.personal.cta': 'Criar uma conta gratuita',
  'grid.orgOnly': 'Só organizações',
  'grid.enDesarrollo': 'Em desenvolvimento',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB por utilizador',
  'home.pricing.paidBody':
    'Para organizações, e também para uma só pessoa. A suite completa, com o Sytue ERP e o Sytue Admin para gerir a sua organização.',
  'home.pricing.thirdParty':
    'Todos os planos permitem ligar o seu próprio armazenamento: Google Drive, Microsoft OneDrive, um bucket S3 (Scaleway, OVH, Hetzner, Wasabi, MinIO), um servidor WebDAV como o Nextcloud ou o Internxt, ou o seu NAS nas suas instalações —sem portas abertas, através de um túnel da Sytue—. As cópias de segurança e a recuperação dos dados guardados em armazenamento ligado são da responsabilidade do cliente, e a Sytue® fica exonerada de qualquer responsabilidade pela perda de dados em armazenamento ligado ao Sytue®.',
  'home.pricing.soon': 'Em breve',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB por utilizador',
  'home.pricing.enterprise.support': 'Com Suporte Premium.',
  'home.pricing.education': 'Instituições de ensino: 50% de desconto no Pro.',
};
