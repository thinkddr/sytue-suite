/**
 * Textos em português do núcleo do Sytue Web (web).
 * Prefixos: nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Não foi possível obter o endereço público do site.',
  'nav.title': 'Web',
  'nav.sites': 'Os meus sites',
  'nav.new': 'Novo site',
  'nav.settings': 'Definições',

  'sites.title': 'Os meus sites',
  'sites.empty': 'Ainda não criou nenhum site.',
  'sites.create': 'Criar site',
  'sites.status.published': 'Publicado',
  'sites.status.draft': 'Rascunho',
  'sites.restore': 'Restaurar',
  'sites.trash': 'Sites no lixo',
  'sites.trash_help':
    'Os sites eliminados são conservados durante 30 dias antes de serem apagados definitivamente.',
  'sites.open_editor': 'Abrir o editor',
  'sites.manage': 'Gerir',
  'sites.last_updated': 'Atualizado a {fecha}',
  'sites.filter_all': 'Todos',
  'sites.filter_published': 'Publicados',
  'sites.filter_draft': 'Rascunhos',
  'sites.count': { one: '{count} site', many: '{count} sites', other: '{count} sites' },
  'sites.empty_help':
    'Escolha um modelo ou comece com uma página em branco. Pode publicá-lo quando quiser.',
  'sites.filter_label': 'Filtrar por estado',
  'sites.filter_empty': 'Nenhum site corresponde a este filtro.',
  'sites.loading': 'A carregar os sites',
  'sites.load_error': 'Não foi possível carregar os sites',
  'sites.load_error_help': 'Verifique a ligação e tente de novo.',
  'sites.retry': 'Tentar de novo',
  'sites.restore_error': 'Não foi possível restaurar o site. Tente de novo.',
  'sites.days_left': {
    one: 'falta {count} dia',
    many: 'faltam {count} dias',
    other: 'faltam {count} dias',
  },

  'new.title': 'Criar um site novo',
  'new.template_step': 'Escolha um modelo',
  'new.details_step': 'Dados do site',
  'new.name_label': 'Nome do site',
  'new.slug_label': 'Identificador (slug)',
  'new.languages_label': 'Idiomas do site',
  'new.submit': 'Criar e abrir o editor',
  'new.step_template': 'Modelo',
  'new.step_info': 'Informação básica',
  'new.step_languages': 'Idiomas',
  'new.primary_language': 'Idioma principal',
  'new.step_review': 'Rever',
  'new.details_help':
    'Dê um nome ao seu site. Preparamos o endereço automaticamente e pode ligar o seu próprio domínio mais tarde.',
  'new.review_heading': 'Tudo pronto para começar',
  'new.review_help': 'Reveja estas opções antes de criar o site.',
  'new.draft_notice':
    'É criado um rascunho privado. No editor pode alterar os textos, as fotografias e o design. Depois ajudamos a rever, a ligar o seu domínio e a publicar.',
  'new.template_blank': 'Em branco',
  'new.template_blank_desc': 'Comece com uma página vazia e crie o design à sua maneira.',
  'new.languages_notice': 'Mais idiomas em breve',
  'new.slug_available': 'Identificador disponível',
  'new.slug_unavailable': 'Este identificador não está disponível',
  'new.slug_checking': 'A verificar o identificador…',
  'new.next': 'Seguinte',
  'new.back': 'Anterior',
  'new.templates_error':
    'Não foi possível carregar os modelos. Pode começar com uma página em branco.',
  'new.name_placeholder': 'A minha loja online',
  'new.slug_placeholder': 'a-minha-loja',
  'new.slug_help': 'Só minúsculas, números e hífenes. Forma o endereço do site.',
  'new.slug_unverified':
    'Não foi possível verificar o identificador agora; será validado ao criar o site.',
  'new.main_language': 'Idioma principal',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Ativar a versão bilingue ({idioma})',
  'new.create_error': 'Não foi possível criar o site. Tente de novo.',
  'new.err_site_limit':
    'A sua organização já tem 20 sites ou já criou 5 hoje. Elimine um ou volte amanhã.',
  'new.cancel': 'Cancelar',
  'new.progress': 'Passos para criar o site',
  'new.go_to_step': 'Ir para o passo {n}',
  'new.step_of': 'Passo {n} de {total}',
  'app_settings.title': 'Definições do Sytue Web',
  'new.search_title': 'Comece pelo seu tipo de negócio',
  'new.search_help':
    'Escreva a que se dedica ou escolha um grupo. Cada modelo já traz os textos da sua atividade: só muda o nome, o telefone, os preços e as fotografias.',
  'new.search_label': 'O seu tipo de negócio',
  'new.search_placeholder': 'Cabeleireiro, café, canalizador, loja de roupa…',
  'new.all_groups': 'Todos',
  'new.results': { one: '{count} modelo', many: '{count} modelos', other: '{count} modelos' },
  'new.no_results': 'Não há nenhum modelo para «{q}».',
  'new.no_results_help':
    'Experimente outra palavra (por exemplo, «loja» ou «clínica») ou comece com uma página em branco.',
  'new.clear_search': 'Limpar a pesquisa',
  'new.select': 'Escolher este modelo',
  'new.selected': 'Escolhido',
  'new.preview': 'Pré-visualização',
  'new.preview_close': 'Fechar',
  'new.preview_loading': 'A carregar o modelo…',
  'new.preview_error': 'Não foi possível carregar a pré-visualização.',
  'new.preview_desktop': 'Computador',
  'new.preview_mobile': 'Telemóvel',
  'new.preview_pages': 'Páginas do modelo',
  'new.preview_note':
    'Vista aproximada: as fotografias são marcadores que vai substituir pelas suas, e os dados de exemplo mudam-se no editor.',
  'new.badge_store': 'Com loja',
  'new.badge_custom': 'Personalizado',
  'new.cat.belleza': 'Beleza e bem-estar',
  'new.cat.comida': 'Comida e bebida',
  'new.cat.comercio': 'Lojas e comércio',
  'new.cat.oficios': 'Ofícios e casa',
  'new.cat.motor': 'Automóvel',
  'new.cat.profesionales': 'Empresas e profissionais',
  'new.cat.salud': 'Saúde',
  'new.cat.deporte': 'Desporto',
  'new.cat.educacion': 'Educação e formação',
  'new.cat.creativos': 'Criatividade e eventos',
  'new.cat.turismo': 'Turismo e alojamento',
  'new.cat.local': 'Produtos locais',
  'new.cat.comunidad': 'Associações e comunidade',
  'new.cat.web': 'Páginas simples',
  'new.cat.otras': 'Outras',

  'publish.title': 'Publicar o site',
  'publish.action': 'Publicar',
  'publish.checklist': 'Lista de verificação antes de publicar',
  'publish.preview': 'Pré-visualização',

  'versions.title': 'Versões',
  'versions.create': 'Criar versão',
  'versions.restore': 'Restaurar esta versão',
  'versions.empty': 'Ainda não há versões guardadas.',
  'versions.name': 'Nome da versão',
  'versions.rev': 'Revisão',
  'versions.restored': 'Versão restaurada',

  'domains.title': 'Domínios',
  'domains.add': 'Adicionar domínio',

  'analytics.title': 'Estatísticas',
  'analytics.visits': 'Visitas',

  'export.title': 'Exportar',
  'export.origins_label': 'Onde vai alojar este código (origens permitidas)',
  'export.origins_help':
    'Introduza um URL https:// por linha a partir do qual será chamada a API pública.',
  'export.schemes_label': 'Esquemas da app (para aplicações nativas)',
  'export.schemes_help':
    'Esquemas de URL personalizados (um por linha), por exemplo: aminhaapp, sytue.',
  'export.save': 'Guardar as definições de exportação',
  'export.saved': 'Definições de exportação guardadas',

  'legal.title': 'Dados legais',
  'legal.holder_name': 'Titular ou denominação social',
  'legal.tax_id': 'NIF / NIPC',
  'legal.country': 'País',
  'legal.street': 'Morada (rua e número)',
  'legal.postal_code': 'Código postal',
  'legal.city': 'Localidade',
  'legal.region': 'Distrito ou região',
  'legal.email': 'Email de contacto legal',
  'legal.phone': 'Telefone',
  'legal.registry': 'Dados do registo comercial (livro, volume, folha)',
  'legal.privacy_contact': 'Contacto de privacidade (EPD)',
  'legal.save': 'Guardar os dados legais',
  'legal.saved': 'Dados legais guardados',

  'settings.title': 'Definições do site',
  'settings.name': 'Nome do site',
  'settings.slug': 'Identificador (slug)',
  'settings.analytics': 'Ativar as estatísticas integradas',
  'settings.analytics_help':
    'Conta as visitas às páginas respeitando a privacidade, sem cookies de terceiros.',
  'settings.save': 'Guardar definições',
  'settings.saved': 'Definições guardadas',
  'settings.danger_zone': 'Zona de perigo',
  'settings.delete_title': 'Eliminar este site',
  'settings.delete_desc':
    'O site passa para o lixo durante 30 dias; durante esse tempo pode restaurá-lo.',
  'settings.delete_confirm_prompt': 'Escreva o nome do site ({name}) para confirmar:',
  'settings.delete_button': 'Eliminar site',

  'errors.not_found': 'Site não encontrado',
  'errors.unauthorized': 'Não tem permissão para ver este site',

  // Ficha do site (islands/SitioDetalleApp.tsx, formularios/Bandeja.tsx, tienda/Pedidos.tsx)
  'ficha.page_title': 'Ficha do site',
  'ficha.back': 'Voltar aos meus sites',
  'ficha.sections': 'Secções do site',
  'ficha.loading': 'A carregar o site',
  'ficha.load_error': 'Não foi possível carregar o site',
  'ficha.open_site': 'Abrir o site',
  'ficha.not_found_help': 'Pode ter sido eliminado ou pode não ter acesso a ele.',

  'publish.tab': 'Publicar',
  'publish.intro':
    'Reveja a lista e publique quando estiver tudo pronto. Uma pré-visualização não muda o que o público vê.',
  'publish.status': 'Estado atual',
  'publish.check_ok': 'Pronto',
  'publish.check_missing': 'Em falta',
  'publish.check_legal': 'Dados legais completos',
  'publish.check_legal_missing': 'Em falta: {campos}',
  'publish.check_legal_fix': 'Completar',
  'publish.check_demo': 'Sem textos de exemplo do modelo',
  'publish.check_ir': 'Páginas bem formadas',
  'publish.check_server':
    'Ao publicar, também se verifica que não ficam textos de exemplo do modelo e que as páginas estão bem formadas.',
  'publish.done_preview': 'Pré-visualização criada.',
  'publish.done_live': 'Site publicado.',
  'publish.open_preview': 'Abrir a pré-visualização',
  'publish.err_legal':
    'Faltam dados legais para publicar. Complete-os no separador «Dados legais».',
  'publish.err_demo':
    'O site ainda tem textos de exemplo do modelo. Substitua-os no editor e publique de novo.',
  'publish.err_ir':
    'Há uma página que não está bem formada. Abra-a no editor para ver o que é preciso corrigir.',
  'publish.err_previews':
    'Já existem 5 pré-visualizações ativas. Aguarde que uma expire para criar outra.',
  'publish.err_too_large':
    'O site ultrapassa o tamanho máximo que se pode publicar (64 MiB ou 5000 ficheiros). Reduza as imagens ou os ficheiros.',
  'publish.err_forbidden': 'Só quem administra a organização pode publicar o site.',
  'publish.err_conflict':
    'Não foi possível publicar porque o site mudou ou está suspenso. Recarregue a página e tente de novo.',
  'publish.err_invalid': 'A publicação não é válida. Reveja o conteúdo do site no editor.',
  'publish.err_generic': 'Não foi possível publicar o site. Tente de novo.',

  'versions.intro':
    'Uma versão é uma cópia do rascunho à qual pode voltar. Crie uma antes de uma alteração grande.',
  'versions.name_placeholder': 'Antes de mudar a página inicial',
  'versions.manual_name': 'Versão manual',
  'versions.unnamed': 'Sem nome',
  'versions.loading': 'A carregar as versões',
  'versions.load_error': 'Não foi possível carregar as versões',
  'versions.empty_help': 'Quando criar uma versão ou publicar o site, ela aparece aqui.',
  'versions.created': 'Versão criada',
  'versions.create_error': 'Não foi possível criar a versão. Tente de novo.',
  'versions.restore_error': 'Não foi possível restaurar a versão. Tente de novo.',
  'versions.restore_confirm':
    'O rascunho volta ao estado de «{nombre}». Antes disso, o estado atual é guardado como «Antes de restaurar», para que possa voltar a ele.',
  'versions.kind.manual': 'Manual',
  'versions.kind.publish': 'Publicação',
  'versions.kind.preview': 'Pré-visualização',
  'versions.kind.export': 'Exportação',
  'versions.kind.before_restore': 'Antes de restaurar',

  'domains.intro':
    'Ligue um domínio próprio. Depois de o adicionar, crie no seu fornecedor de DNS os registos indicados e verifique a configuração.',
  'domains.host_label': 'Domínio',
  'domains.host_placeholder': 'www.exemplo.pt',
  'domains.host_help': 'Só o nome do domínio, sem https:// nem barras.',
  'domains.empty': 'Ainda não há domínios próprios configurados.',
  'domains.loading': 'A carregar os domínios',
  'domains.load_error': 'Não foi possível carregar os domínios',
  'domains.primary': 'Principal',
  'domains.status.pending': 'Verificação pendente',
  'domains.status.verified': 'Verificado',
  'domains.status.active': 'Ativo',
  'domains.status.failed': 'Com erro',
  'domains.dns_title': 'Registos DNS',
  'domains.dns_txt': 'TXT (verificação)',
  'domains.dns_cname': 'CNAME (subdomínios como www)',
  'domains.dns_a': 'A (domínio raiz)',
  'domains.check': 'Verificar DNS e HTTPS',
  'domains.make_primary': 'Tornar principal',
  'domains.remove': 'Remover',
  'domains.remove_title': 'Remover domínio',
  'domains.remove_confirm':
    '«{host}» deixa de apontar para este site. Para o voltar a usar, terá de o adicionar de novo.',
  'domains.added':
    'Domínio adicionado. Crie no seu fornecedor de DNS os registos que aparecem abaixo.',
  'domains.updated': 'Domínios atualizados.',
  'domains.action_error': 'Não foi possível concluir a ação. Tente de novo.',
  'domains.err_invalid': 'O nome do domínio não é válido.',
  'domains.err_reserved': 'Esse domínio está reservado e não pode ser usado.',
  'domains.err_taken': 'Esse domínio já está a ser usado noutro site.',
  'domains.err_limit': 'Atingiu o número máximo de domínios deste site.',
  'domains.err_suspended': 'O site está suspenso e não admite alterações de domínio.',
  'domains.err_forbidden': 'Só quem administra a organização pode gerir domínios.',
  'domains.err_generic': 'Não foi possível concluir a operação com o domínio. Tente de novo.',
  'domains.start_title': 'Ligue o seu domínio, passo a passo',
  'domains.start_help':
    'Introduza o domínio que quer usar. Vai precisar de acesso à configuração de DNS no fornecedor onde o comprou.',
  'domains.progress': 'Progresso da ligação do domínio',
  'domains.step.registered': 'Adicionado ao Sytue Web',
  'domains.step.dns': 'DNS verificado',
  'domains.step.https': 'HTTPS pronto',
  'domains.step.primary': 'Endereço principal',
  'domains.step.done': 'Concluído',
  'domains.step.pending': 'Pendente',
  'domains.status.review': 'Precisa de revisão',
  'domains.record_name': 'Nome do registo',
  'domains.record_value': 'Valor do registo',
  'domains.routing_title': 'Endereço do site',
  'domains.dns_help':
    'Abra a configuração de DNS do seu fornecedor e adicione estes dois registos. Copie cada nome e valor exatamente como aparecem.',
  'domains.dns_names_help':
    'Alguns fornecedores usam @ para o domínio raiz ou apenas www para esse subdomínio. O nome completo do registo aparece acima.',
  'domains.routing_legacy_help':
    'Use A para o domínio raiz ou CNAME para um subdomínio como www, com o destino correspondente indicado pelo serviço.',
  'domains.dns_propagation':
    'Depois de guardar os registos, clique em «Verificar DNS e HTTPS». Se ainda não aparecerem, aguarde que o fornecedor propague a alteração e verifique de novo. Mantenha os registos do email e de outros serviços.',
  'domains.records_unavailable':
    'O serviço ainda não forneceu todos os dados deste registo. Não altere o DNS até estarem disponíveis.',
  'domains.copy': 'Copiar',
  'domains.copied': 'Copiado',
  'domains.copy_field': 'Copiar {campo}',
  'domains.copy_error':
    'Não foi possível copiar para a área de transferência. Selecione o valor do campo e copie-o manualmente.',
  'domains.last_check': 'Última verificação: {fecha}',
  'domains.publish_help':
    'Quando o endereço principal estiver pronto, publique o site de novo para que as ligações e os redirecionamentos usem o seu domínio.',
  'domains.publish_action': 'Ir para publicar',
  'domains.detail.pending':
    'O domínio já foi adicionado. Configure os registos DNS e verifique a ligação para continuar.',
  'domains.detail.txt_missing':
    'O registo TXT ainda não foi encontrado. Adicione-o no seu fornecedor de DNS e verifique de novo depois de a alteração se propagar.',
  'domains.detail.txt_mismatch':
    'O valor do TXT não coincide. Substitua-o pelo valor exato que aparece neste ecrã.',
  'domains.detail.dns_unreachable':
    'Não foi possível consultar o DNS deste domínio. Reveja o domínio e os seus servidores de nomes e verifique de novo.',
  'domains.detail.routing':
    'O domínio ainda não aponta para o site. Reveja o registo A ou CNAME indicado e verifique de novo.',
  'domains.detail.certificate':
    'Os registos DNS estão verificados. O certificado HTTPS ainda não está pronto; verifique de novo dentro de alguns minutos.',
  'domains.detail.active': 'O DNS e a ligação HTTPS estão verificados. O domínio está pronto.',
  'domains.detail.unknown':
    'A última verificação precisa de revisão. Reveja os registos e verifique a ligação de novo.',

  'analytics.intro':
    'Durante o piloto, as visitas são uma estimativa diária; não correspondem a pessoas únicas no período.',
  'analytics.visitors': 'Visitas estimadas',
  'analytics.loading': 'A carregar as estatísticas',
  'analytics.load_error': 'Não foi possível carregar as estatísticas',
  'analytics.unavailable':
    'O serviço de estatísticas não está disponível neste momento. Tente de novo dentro de alguns minutos.',
  'analytics.disabled': 'As estatísticas estão desativadas',
  'analytics.disabled_help': 'Ative-as nas definições do site para começar a contar visitas.',
  'analytics.go_settings': 'Ir para as definições',
  'analytics.refresh': 'Atualizar',

  'export.intro':
    'Se vai alojar o código do site fora do Sytue, indique a partir de onde será chamada a API pública.',
  'export.origins_placeholder': 'https://aminhaloja.exemplo.pt',
  'export.schemes_placeholder': 'aminhaapp',
  'export.origins_invalid':
    'Cada linha tem de ser um endereço https:// válido (a linha {n} não é).',
  'export.schemes_invalid':
    'Cada linha tem de ser um esquema válido: começa por uma letra e só tem letras, algarismos, «+», «-» ou «.» (a linha {n} não cumpre).',
  'export.err_generic': 'Não foi possível guardar as definições. Tente de novo.',

  'legal.intro': 'São obrigatórios para publicar o site.',
  'legal.required_note': 'Os campos assinalados com * são obrigatórios.',
  'legal.saved_incomplete': 'Dados guardados. Para publicar, ainda falta: {campos}.',
  'legal.country_help': 'Código de duas letras, por exemplo PT.',
  'legal.registry_help':
    'Obrigatório para sociedades espanholas (NIF que começa por A, B, C, D ou F).',
  'legal.ph_holder': 'Exemplo, Lda.',
  'legal.ph_street': 'Rua Augusta, 12, 1.º',
  'legal.ph_city': 'Lisboa',
  'legal.ph_email': 'legal@exemplo.pt',
  'legal.ph_privacy': 'epd@exemplo.pt',
  'legal.ph_registry': 'Registo Mercantil de Madrid, volume 1234, fólio 56, folha M-789',
  'legal.err_generic': 'Não foi possível guardar os dados legais. Tente de novo.',

  'settings.tab': 'Definições',
  'settings.intro': 'Nome, endereço e opções gerais do site.',
  'settings.slug_help':
    'Só minúsculas, algarismos e hífenes. Se o alterar, o endereço do site muda.',
  'settings.admin_only': 'Só quem administra a organização pode alterar isto.',
  'settings.name_required': 'O nome do site não pode ficar vazio.',
  'settings.err_slug_taken': 'Esse identificador já é usado por outro site.',
  'settings.err_invalid': 'Reveja o nome, o identificador e o endereço: algum não é válido.',
  'settings.err_generic': 'Não foi possível guardar as definições. Tente de novo.',
  'settings.delete_error': 'Não foi possível eliminar o site. Tente de novo.',

  'forms.empty': 'Ainda não há respostas',
  'forms.empty_help': 'Quando alguém enviar um formulário do seu site, vai vê-lo aqui.',
};
