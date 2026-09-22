// glossary.ts — el diccionario de la app: la jerga de ccp explicada en el sitio
// donde aparece.
//
// Cada término dice tres cosas, siempre en este orden, porque son las tres
// preguntas que se hace quien lo ve por primera vez: qué es, por qué existe y
// cómo funciona. Los tooltips (components/Help.tsx) y la pantalla Glosario leen
// de aquí: un solo texto por concepto, o dos sitios acabarían explicando lo
// mismo de dos maneras.
//
// El texto vive aquí en los dos idiomas en vez de en i18n_en.ts: son párrafos de
// contenido, no frases de interfaz, y tenerlos juntos obliga a mantenerlos a la
// par al editar uno.

import { getLang } from './i18n';

export interface GlossaryText {
  term: string;
  what: string;
  why: string;
  how: string;
}

export type GlossaryGroup = 'cuentas' | 'conversaciones' | 'rotacion' | 'configuracion' | 'desktop' | 'historial' | 'app';

interface Entry {
  group: GlossaryGroup;
  es: GlossaryText;
  en: GlossaryText;
}

const G: Record<string, Entry> = {
  // --- Cuentas -------------------------------------------------------------
  cuenta: {
    group: 'cuentas',
    es: {
      term: 'Cuenta (perfil)',
      what: 'Una identidad con la que corre Claude: una cuenta de Anthropic, un proveedor compatible (DeepSeek, Kimi, GLM…) o default, tu Claude de siempre.',
      why: 'Para usar varias cuentas en la misma máquina sin cerrar y abrir sesión: el trabajo va con una, lo personal con otra, y cada una conserva su login, su configuración y sus conversaciones.',
      how: 'Cada cuenta tiene su propia carpeta de Claude Code (su cc-home). ccp la activa por terminal y por carpeta, nunca de forma global: abrir una terminal en otra carpeta puede cambiar de cuenta sola.',
    },
    en: {
      term: 'Account (profile)',
      what: 'An identity Claude runs as: an Anthropic account, a compatible provider (DeepSeek, Kimi, GLM…) or default, your usual Claude.',
      why: 'To use several accounts on one machine without signing out and in: work goes with one, personal with another, and each keeps its own login, configuration and conversations.',
      how: 'Each account has its own Claude Code folder (its cc-home). ccp activates it per terminal and per folder, never globally: opening a terminal in another folder can switch accounts on its own.',
    },
  },
  default: {
    group: 'cuentas',
    es: {
      term: 'default',
      what: 'Tu Claude de siempre: la sesión de ~/.claude, tal como la tenías antes de ccp.',
      why: 'Para que ccp no se apropie de nada: lo que no tiene regla sigue funcionando exactamente igual que sin ccp.',
      how: 'Es la cuenta de toda carpeta sin regla. No se renombra ni se borra, su configuración es la capa global y su ventana de Desktop es tu Claude normal.',
    },
    en: {
      term: 'default',
      what: 'Your usual Claude: the ~/.claude session, exactly as you had it before ccp.',
      why: 'So ccp takes nothing over: whatever has no rule keeps working exactly as it did without ccp.',
      how: 'It is the account of every folder without a rule. It can’t be renamed or deleted, its configuration is the global layer and its Desktop window is your normal Claude.',
    },
  },
  oficial: {
    group: 'cuentas',
    es: {
      term: 'Cuenta de Anthropic',
      what: 'Una cuenta de claude.ai (Pro, Max, Team…) con su propio login.',
      why: 'Cada suscripción tiene su propio límite de uso; separarlas permite repartir el trabajo y que una siga cuando la otra se agota.',
      how: 'Se inicia sesión una vez con «Iniciar sesión» (ccp profile login), dentro de Claude Code. Puede tener ventana de Desktop propia y prestar o recibir conversaciones.',
    },
    en: {
      term: 'Anthropic account',
      what: 'A claude.ai account (Pro, Max, Team…) with its own login.',
      why: 'Each subscription has its own usage limit; keeping them apart lets you split the work and have one continue when the other runs out.',
      how: 'You sign in once with “Sign in” (ccp profile login), inside Claude Code. It can have its own Desktop window and lend or receive conversations.',
    },
  },
  proveedor: {
    group: 'cuentas',
    es: {
      term: 'Proveedor',
      what: 'Un servicio compatible con la API de Anthropic (DeepSeek, Kimi, GLM…) que Claude Code usa en lugar de Anthropic.',
      why: 'Para tener un respaldo más barato o sin los límites de tu suscripción, con la misma herramienta.',
      how: 'Se guarda su endpoint, sus modelos y una API key (en un archivo aparte con permisos 600, nunca en ccp.yaml). No tiene ventana de Desktop: Desktop solo habla con Anthropic.',
    },
    en: {
      term: 'Provider',
      what: 'A service compatible with Anthropic’s API (DeepSeek, Kimi, GLM…) that Claude Code uses instead of Anthropic.',
      why: 'To have a cheaper backup, or one without your subscription’s limits, with the same tool.',
      how: 'It stores its endpoint, its models and an API key (in a separate file with 600 permissions, never in ccp.yaml). It has no Desktop window: Desktop only talks to Anthropic.',
    },
  },
  resumen: {
    group: 'cuentas',
    es: {
      term: 'Resumen',
      what: 'La portada de una cuenta: si tiene acceso, cuánto uso le queda, un vistazo a lo demás y la zona de riesgo.',
      why: 'Para saber de un golpe si la cuenta está lista para trabajar antes de entrar al detalle.',
      how: 'Cada fila de «De un vistazo» abre la pestaña de ESTA cuenta. Renombrar y borrar están aquí, en la zona de riesgo, y piden confirmación.',
    },
    en: {
      term: 'Overview',
      what: 'An account’s front page: whether it has access, how much usage it has left, a glance at the rest and the danger zone.',
      why: 'To tell at once whether the account is ready to work before going into the details.',
      how: 'Each row of “At a glance” opens THIS account’s tab. Rename and delete live here, in the danger zone, and ask for confirmation.',
    },
  },
  carpetas: {
    group: 'cuentas',
    es: {
      term: 'Carpetas (reglas)',
      what: 'Las reglas que dicen qué cuenta usa cada carpeta: «~/trabajo usa work».',
      why: 'Para no tener que acordarte de cambiar de cuenta: entras en la carpeta y ya estás en la correcta.',
      how: 'Una carpeta usa la cuenta de su regla más cercana hacia arriba; sin ninguna, default. La terminal cambia de cuenta sola al hacer cd, porque ccp mira la carpeta en cada prompt.',
    },
    en: {
      term: 'Folders (rules)',
      what: 'The rules that say which account each folder uses: “~/work uses work”.',
      why: 'So you don’t have to remember to switch accounts: you enter the folder and you’re already on the right one.',
      how: 'A folder uses the account of its nearest rule upwards; with none, default. The terminal switches account on its own when you cd, because ccp checks the folder at every prompt.',
    },
  },
  excepcion: {
    group: 'cuentas',
    es: {
      term: 'Excepción',
      what: 'Una regla dentro de la carpeta de otra regla, que la contradice para una subcarpeta.',
      why: 'Para casos como «todo ~/trabajo es work salvo ~/trabajo/personal».',
      how: 'Gana la regla más profunda. Una excepción a default sirve para sacar una subcarpeta de la cuenta de su carpeta padre.',
    },
    en: {
      term: 'Exception',
      what: 'A rule inside another rule’s folder that overrides it for a subfolder.',
      why: 'For cases like “all of ~/work is work, except ~/work/personal”.',
      how: 'The deepest rule wins. An exception to default takes a subfolder out of its parent folder’s account.',
    },
  },
  carpeta_contexto: {
    group: 'app',
    es: {
      term: 'Carpeta en contexto',
      what: 'La carpeta que eliges arriba, en la barra de la app.',
      why: 'Muchas preguntas dependen de dónde estás: qué cuenta te toca, qué cadena rota, qué conversaciones hay aquí.',
      how: 'Las pantallas de General la usan para responder «aquí». Dentro de una cuenta manda la cuenta, no la carpeta.',
    },
    en: {
      term: 'Context folder',
      what: 'The folder you pick at the top of the app.',
      why: 'Many questions depend on where you are: which account you get, which chain rotates, which conversations live here.',
      how: 'The General screens use it to answer “here”. Inside an account, the account rules, not the folder.',
    },
  },

  // --- Conversaciones ---------------------------------------------------------
  conversacion: {
    group: 'conversaciones',
    es: {
      term: 'Conversación',
      what: 'Una sesión de Claude Code: todo lo que se habló, guardado como un archivo (transcript) dentro de la cuenta con la que se hizo.',
      why: 'Es lo que se quiere conservar y poder seguir, aunque cambies de cuenta o de ventana.',
      how: 'Vive en la carpeta de su cuenta y se identifica por un uuid; la app la busca por título. Si está en varias cuentas, son copias independientes con el mismo uuid.',
    },
    en: {
      term: 'Conversation',
      what: 'A Claude Code session: everything said, saved as a file (transcript) inside the account it was made with.',
      why: 'It’s what you want to keep and be able to continue, even if you switch account or window.',
      how: 'It lives in its account’s folder and is identified by a uuid; the app finds it by title. If it’s in several accounts, those are independent copies with the same uuid.',
    },
  },
  conversaciones: {
    group: 'conversaciones',
    es: {
      term: 'Conversaciones',
      what: 'La lista de todas las sesiones, de la terminal y de Desktop, de todas las cuentas o de la que estés mirando.',
      why: 'Sin ella tendrías que saber en qué cuenta y en qué carpeta quedó cada una.',
      how: 'Se filtra por carpeta, por Desktop, por prestadas o archivadas y se busca por título. Desde cada fila se mueve a otra cuenta; la vista Préstamos enseña las que están fuera de casa.',
    },
    en: {
      term: 'Conversations',
      what: 'The list of every session, from the terminal and from Desktop, of every account or the one you’re looking at.',
      why: 'Without it you’d have to know which account and folder each one ended up in.',
      how: 'Filter by folder, by Desktop, by loaned or archived, and search by title. Each row can be moved to another account; the Loans view shows the ones away from home.',
    },
  },
  prestamo: {
    group: 'conversaciones',
    es: {
      term: 'Préstamo',
      what: 'Una conversación que sigue en otra cuenta sin dejar de ser de la suya: se lleva, se trabaja allí y vuelve.',
      why: 'Para no perder el hilo cuando una cuenta se queda sin uso: la conversación continúa en otra con todo su contexto.',
      how: 'Lo crea «Mover» en modo préstamo o la rotación automática. «Terminar y traer» la devuelve a casa con lo que se añadió fuera. Un préstamo «zombi» es uno cuyo transcript ya no está en el destino: solo queda descartar el marcador.',
    },
    en: {
      term: 'Loan',
      what: 'A conversation that continues in another account while still belonging to its own: it goes, gets worked on there, and comes back.',
      why: 'So you don’t lose the thread when an account runs out of usage: the conversation continues in another one with all its context.',
      how: '“Move” in loan mode or automatic rotation creates it. “End and bring back” returns it home with what was added away. A “zombie” loan is one whose transcript is gone from the destination: all that’s left is to discard the marker.',
    },
  },
  mover: {
    group: 'conversaciones',
    es: {
      term: 'Mover una conversación',
      what: 'Llevar una conversación a otra cuenta, copiándola o prestándola.',
      why: 'Para seguirla en otra cuenta (la tuya se agotó) o en la ventana de Desktop de otra cuenta.',
      how: 'Antes de hacer nada enseña el plan: qué se copia, si el destino ya la tenía y si hay que reiniciar una ventana. Nunca borra el original ni pisa una copia más nueva.',
    },
    en: {
      term: 'Move a conversation',
      what: 'Taking a conversation to another account, by copying or lending it.',
      why: 'To continue it in another account (yours ran out) or in another account’s Desktop window.',
      how: 'Before doing anything it shows the plan: what gets copied, whether the destination already had it and whether a window needs restarting. It never deletes the original or overwrites a newer copy.',
    },
  },

  // --- Rotación -------------------------------------------------------------------
  rotacion: {
    group: 'rotacion',
    es: {
      term: 'Rotación automática',
      what: 'Que una sesión cambie sola de cuenta cuando la suya llega a su límite de uso, y vuelva cuando se libera.',
      why: 'Para que un trabajo largo (o uno que dejas corriendo de noche) no se pare al agotarse una cuenta.',
      how: 'Funciona en las sesiones supervisadas (ccp session). Los sensores avisan del límite, ccp presta la conversación al siguiente respaldo de la cadena y la trae de vuelta cuando la principal se libera.',
    },
    en: {
      term: 'Automatic rotation',
      what: 'A session switching account on its own when its account hits its usage limit, and coming back when it frees up.',
      why: 'So long work (or work you leave running overnight) doesn’t stop when one account runs out.',
      how: 'It works in supervised sessions (ccp session). The sensors report the limit, ccp lends the conversation to the chain’s next backup and brings it back when the primary frees up.',
    },
  },
  cadena: {
    group: 'rotacion',
    es: {
      term: 'Cadena',
      what: 'La lista ordenada de cuentas que respaldan a una cuenta principal.',
      why: 'Para decidir quién toma el relevo y en qué orden, en vez de que sea al azar.',
      how: 'Se prueba de arriba abajo: si el primer respaldo también se agota, salta al siguiente. Se ordena arrastrando.',
    },
    en: {
      term: 'Chain',
      what: 'The ordered list of accounts that back up a primary account.',
      why: 'To decide who takes over and in which order, instead of leaving it to chance.',
      how: 'It’s tried top to bottom: if the first backup runs out too, it jumps to the next. Reorder it by dragging.',
    },
  },
  principal: {
    group: 'rotacion',
    es: {
      term: 'Principal',
      what: 'La cuenta con la que empieza una sesión: la de la carpeta, o la cuenta cuya cadena miras.',
      why: 'Es la cuenta «de casa»: a ella vuelve la conversación en cuanto se puede.',
      how: 'Cuando se agota, la sesión se presta a su cadena; cuando se libera y la sesión está quieta un rato, vuelve sola.',
    },
    en: {
      term: 'Primary',
      what: 'The account a session starts with: the folder’s, or the account whose chain you’re looking at.',
      why: 'It’s the “home” account: the conversation goes back to it as soon as it can.',
      how: 'When it runs out, the session is lent to its chain; when it frees up and the session has been idle for a while, it comes back on its own.',
    },
  },
  respaldo: {
    group: 'rotacion',
    es: {
      term: 'Respaldo',
      what: 'Una cuenta de la cadena, dispuesta a seguir la conversación cuando la principal se agota.',
      why: 'Para tener a dónde ir cuando la principal no puede más.',
      how: 'Necesita acceso (login o key) y, a ser posible, sensores. Uno bloqueado o sin permiso se salta.',
    },
    en: {
      term: 'Backup',
      what: 'An account in the chain, ready to continue the conversation when the primary runs out.',
      why: 'So there’s somewhere to go when the primary can’t take more.',
      how: 'It needs access (login or key) and, ideally, sensors. A blocked or unauthorized one is skipped.',
    },
  },
  cadena_propia: {
    group: 'rotacion',
    es: {
      term: 'Cadena propia / heredada',
      what: 'Propia: la cuenta tiene su lista de respaldos. Heredada: usa la lista compartida de su política.',
      why: 'La compartida sirve para empezar rápido; la propia, para decir «esta cuenta solo se apoya en estas».',
      how: 'Editar la cadena de una cuenta la convierte en propia. «Volver a heredar» la devuelve a la compartida. Una cadena propia vacía significa «no presta a nadie».',
    },
    en: {
      term: 'Own / inherited chain',
      what: 'Own: the account has its own backup list. Inherited: it uses its policy’s shared list.',
      why: 'The shared one gets you started fast; an own one lets you say “this account relies only on these”.',
      how: 'Editing an account’s chain makes it its own. “Inherit again” returns it to the shared one. An empty own chain means “lends to nobody”.',
    },
  },
  permisos: {
    group: 'rotacion',
    es: {
      term: 'Permisos de préstamo',
      what: 'Un mapa opcional de qué respaldos puede usar cada principal (allow_from).',
      why: 'Por cumplimiento: quizá una cuenta de empresa no deba pasar su trabajo a una personal.',
      how: 'Sin mapa, todo lo de la cadena vale. Con mapa, cada principal necesita su entrada: si falta, ningún respaldo pasa y la rotación no ocurre, en silencio. Por eso la app lo avisa.',
    },
    en: {
      term: 'Loan permissions',
      what: 'An optional map of which backups each primary may use (allow_from).',
      why: 'For compliance: a company account perhaps shouldn’t hand its work to a personal one.',
      how: 'Without a map, the whole chain is allowed. With one, each primary needs its own entry: if it’s missing, no backup gets through and rotation silently doesn’t happen. That’s why the app warns about it.',
    },
  },
  politica: {
    group: 'rotacion',
    es: {
      term: 'Política',
      what: 'El conjunto de parámetros de la rotación: cuándo rotar, cuánto esperar, cuántos préstamos y cuándo volver.',
      why: 'Para ajustar cómo de agresiva es la rotación sin tocar las cadenas.',
      how: 'Hay una por defecto y puede haber más con nombre; una cuenta puede fijar la suya. Los parámetros los comparten todas las cuentas que usan esa política.',
    },
    en: {
      term: 'Policy',
      what: 'The set of rotation parameters: when to rotate, how long to wait, how many loans and when to come back.',
      why: 'To tune how aggressive rotation is without touching the chains.',
      how: 'There’s a default one and there can be more, named; an account can pin its own. The parameters are shared by every account using that policy.',
    },
  },
  threshold: {
    group: 'rotacion',
    es: {
      term: 'Rotar al llegar al (umbral)',
      what: 'El porcentaje de uso de la ventana al que la sesión empieza a cambiar de cuenta.',
      why: 'Para rotar antes de chocar con el límite, en vez de esperar a que falle.',
      how: 'Lo mide el sensor de la barra de estado. Al cruzarlo se presta la conversación aunque todavía no haya fallado nada.',
    },
    en: {
      term: 'Rotate at (threshold)',
      what: 'The window usage percentage at which the session starts switching account.',
      why: 'To rotate before hitting the limit, instead of waiting for it to fail.',
      how: 'The status-line sensor measures it. Once crossed, the conversation is lent even though nothing has failed yet.',
    },
  },
  min_dwell: {
    group: 'rotacion',
    es: {
      term: 'Tiempo mínimo en una cuenta',
      what: 'Cuánto tiene que pasar en una cuenta antes de un cambio anticipado.',
      why: 'Para que la sesión no salte de cuenta en cuenta ante avisos seguidos.',
      how: 'Solo frena los avisos anticipados del umbral. Ante un límite real se espera como mucho 30 s: no tiene sentido quedarse en una cuenta que ya no responde.',
    },
    en: {
      term: 'Minimum time on an account',
      what: 'How long a session stays on an account before an early switch.',
      why: 'So the session doesn’t hop between accounts on back-to-back warnings.',
      how: 'It only slows down early threshold warnings. On a real limit it waits 30 s at most: there’s no point staying on an account that no longer answers.',
    },
  },
  max_hops: {
    group: 'rotacion',
    es: {
      term: 'Máximo de préstamos por sesión',
      what: 'Cuántas veces puede prestarse una sesión a otra cuenta antes de pararse.',
      why: 'Es el freno de seguridad: evita que una sesión recorra todas tus cuentas sin fin.',
      how: 'Solo cuentan las salidas; volver a casa no gasta préstamo. Al agotarse, la sesión se detiene y espera.',
    },
    en: {
      term: 'Maximum loans per session',
      what: 'How many times a session can be lent to another account before it stops.',
      why: 'It’s the safety brake: it stops a session from going through all your accounts endlessly.',
      how: 'Only outbound moves count; coming home is free. When it runs out, the session stops and waits.',
    },
  },
  return_check: {
    group: 'rotacion',
    es: {
      term: 'Cada cuánto mirar si se puede volver',
      what: 'Cada cuánto comprueba ccp, durante un préstamo, si la principal ya se liberó.',
      why: 'Los sensores avisan cuando una cuenta se agota, nunca cuando otra se libera: alguien tiene que preguntar.',
      how: 'Si la principal está libre y la sesión lleva quieta el tiempo de inactividad, ccp la trae de vuelta a casa.',
    },
    en: {
      term: 'How often to check for a return',
      what: 'How often, during a loan, ccp checks whether the primary has freed up.',
      why: 'Sensors report when an account runs out, never when another frees up: someone has to ask.',
      how: 'If the primary is free and the session has been idle long enough, ccp brings it back home.',
    },
  },
  return_idle: {
    group: 'rotacion',
    es: {
      term: 'Inactividad necesaria para volver',
      what: 'Cuánto tiempo tiene que estar quieta una sesión para que ccp la devuelva a casa.',
      why: 'Volver es reiniciar Claude Code: nadie quiere que pase a mitad de una respuesta.',
      how: 'Se mide por la última escritura del transcript. Con 0 se desactiva la espera.',
    },
    en: {
      term: 'Idle time needed to return',
      what: 'How long a session must be quiet before ccp takes it back home.',
      why: 'Returning means restarting Claude Code: nobody wants it to happen mid-answer.',
      how: 'It’s measured from the transcript’s last write. 0 disables the wait.',
    },
  },
  cooldown: {
    group: 'rotacion',
    es: {
      term: 'Enfriamiento',
      what: 'Cuánto tiempo se da por agotada una cuenta tras llegar a su límite.',
      why: 'Para no volver a probar una cuenta que todavía no se ha recuperado.',
      how: '«Hora de reinicio» usa la hora que la propia cuenta informa; «tiempo fijo» espera siempre lo mismo. El enfriamiento de respaldo se usa cuando no se sabe la hora.',
    },
    en: {
      term: 'Cooldown',
      what: 'How long an account counts as exhausted after hitting its limit.',
      why: 'So an account that hasn’t recovered yet isn’t tried again.',
      how: '“Reset time” uses the time the account itself reports; “fixed time” always waits the same. The fallback cooldown is used when the time is unknown.',
    },
  },
  sensores: {
    group: 'rotacion',
    es: {
      term: 'Sensores',
      what: 'Dos piezas que ccp instala en la configuración de una cuenta: la barra de estado, que mide el uso, y un hook que avisa cuando Claude Code choca con el límite.',
      why: 'Sin ellos la rotación solo se entera del límite cuando ya ha fallado, y la app no puede enseñarte el uso.',
      how: 'Se instalan desde Rotación o Diagnóstico. Nunca rompen Claude Code: si fallan, callan. La pestaña Code de Desktop no ejecuta la barra de estado, así que ahí no miden.',
    },
    en: {
      term: 'Sensors',
      what: 'Two pieces ccp installs in an account’s configuration: the status line, which measures usage, and a hook that fires when Claude Code hits the limit.',
      why: 'Without them rotation only learns about the limit once it has failed, and the app can’t show you usage.',
      how: 'Install them from Rotation or Diagnostics. They never break Claude Code: if they fail, they stay quiet. Desktop’s Code tab doesn’t run the status line, so they don’t measure there.',
    },
  },
  uso: {
    group: 'rotacion',
    es: {
      term: 'Uso (ventanas de 5 h y 7 d)',
      what: 'Cuánto has gastado de los dos límites de una cuenta de Anthropic: el de las últimas 5 horas y el semanal.',
      why: 'Para ver venir el límite antes de que corte el trabajo y elegir con qué cuenta seguir.',
      how: 'Lo informan los sensores mientras Claude Code corre en una terminal con esa cuenta. Cada ventana dice cuándo se reinicia.',
    },
    en: {
      term: 'Usage (5 h and 7 d windows)',
      what: 'How much of an Anthropic account’s two limits you’ve used: the last-5-hours one and the weekly one.',
      why: 'To see the limit coming before it cuts your work and choose which account to continue with.',
      how: 'The sensors report it while Claude Code runs in a terminal with that account. Each window says when it resets.',
    },
  },
  supervisada: {
    group: 'rotacion',
    es: {
      term: 'Sesión supervisada',
      what: 'Claude Code lanzado a través de ccp session, que lo vigila y lo cambia de cuenta si hace falta.',
      why: 'Es la única forma de que la rotación automática ocurra: un claude normal no sabe cambiar de cuenta.',
      how: 'Se abre en una terminal. Al llegar al límite, ccp cierra Claude Code, presta la conversación y lo vuelve a abrir en la siguiente cuenta, donde se había quedado.',
    },
    en: {
      term: 'Supervised session',
      what: 'Claude Code started through ccp session, which watches it and switches its account when needed.',
      why: 'It’s the only way automatic rotation happens: a plain claude can’t switch accounts.',
      how: 'It opens in a terminal. At the limit, ccp closes Claude Code, lends the conversation and reopens it in the next account, right where it left off.',
    },
  },
  dejar_trabajando: {
    group: 'rotacion',
    es: {
      term: 'Dejar trabajando',
      what: 'Seguir una conversación desatendida, en una terminal, con su cuenta y bajo el supervisor, para que cambie sola de cuenta si llega al límite.',
      why: 'Para dejar a Claude programando de noche sin que un límite de uso lo pare a las 3 de la madrugada.',
      how: 'ccp copia la conversación (la original, también la de Desktop, queda intacta), abre una terminal con ccp session y le manda un mensaje al empezar y otro en cada cambio de cuenta, porque nadie estará para escribir. Mantiene la Mac despierta y, si todas las cuentas se agotan, se detiene y dice cuándo se libera cada una.',
    },
    en: {
      term: 'Leave working',
      what: 'Continuing a conversation unattended, in a terminal, with its account and under the supervisor, so it switches account on its own if it hits the limit.',
      why: 'To leave Claude coding overnight without a usage limit stopping it at 3 a.m.',
      how: 'ccp copies the conversation (the original, including a Desktop one, stays untouched), opens a terminal with ccp session and sends it a message at the start and another on every account switch, since nobody will be there to type. It keeps the Mac awake and, if every account runs out, it stops and says when each one frees up.',
    },
  },
  mapa: {
    group: 'rotacion',
    es: {
      term: 'Mapa de cuentas',
      what: 'Un lienzo donde las cuentas son nodos y los respaldos flechas.',
      why: 'Para diseñar la rotación viéndola, en vez de editando listas.',
      how: 'Se conectan cuentas arrastrando. Nada se escribe hasta «Revisar y aplicar», que enseña antes el cambio.',
    },
    en: {
      term: 'Account map',
      what: 'A canvas where accounts are nodes and backups are arrows.',
      why: 'To design rotation by seeing it, instead of editing lists.',
      how: 'Connect accounts by dragging. Nothing is written until “Review and apply”, which shows the change first.',
    },
  },

  // --- Configuración ---------------------------------------------------------
  configuracion: {
    group: 'configuracion',
    es: {
      term: 'Configuración',
      what: 'Todo lo que Claude lee además de tu mensaje: instrucciones (CLAUDE.md), servidores MCP, skills, agentes, comandos, hooks, permisos, variables y ajustes.',
      why: 'Para que cada cuenta trabaje con las herramientas y reglas que le tocan, sin copiar archivos a mano.',
      how: 'Se organiza en capas que se suman. Tú declaras en una capa y ccp la proyecta a los archivos que leen Claude Code y Desktop.',
    },
    en: {
      term: 'Configuration',
      what: 'Everything Claude reads besides your message: instructions (CLAUDE.md), MCP servers, skills, agents, commands, hooks, permissions, variables and settings.',
      why: 'So each account works with the tools and rules it should, without copying files by hand.',
      how: 'It’s organized in layers that add up. You declare in a layer and ccp projects it into the files Claude Code and Desktop read.',
    },
  },
  capa_global: {
    group: 'configuracion',
    es: {
      term: 'Capa global',
      what: 'La configuración de ~/.claude: la base que heredan todas las cuentas.',
      why: 'Para escribir una vez lo que quieres en todas partes.',
      how: 'Cada cuenta recibe la global más lo suyo; si las dos dicen lo mismo, gana la cuenta. default no tiene capa propia: su capa es esta.',
    },
    en: {
      term: 'Global layer',
      what: 'The ~/.claude configuration: the base every account inherits.',
      why: 'To write once what you want everywhere.',
      how: 'Each account gets the global layer plus its own; if both say the same thing, the account wins. default has no layer of its own: this is it.',
    },
  },
  capa_claude_code: {
    group: 'configuracion',
    es: {
      term: 'Capa de la cuenta (Claude Code)',
      what: 'Lo que una cuenta añade o cambia sobre la global: su CLAUDE.md, sus MCP, sus skills, sus variables…',
      why: 'Para que la cuenta del trabajo tenga sus herramientas sin que aparezcan en la personal.',
      how: 'Se guarda en el overlay de la cuenta y ccp lo proyecta a su cc-home. Lo lee Claude Code en la terminal y en la pestaña Code de su ventana de Desktop.',
    },
    en: {
      term: 'Account layer (Claude Code)',
      what: 'What an account adds or changes on top of the global one: its CLAUDE.md, its MCP servers, its skills, its variables…',
      why: 'So the work account has its tools without them showing up in the personal one.',
      how: 'It’s stored in the account’s overlay and ccp projects it into its cc-home. Claude Code reads it in the terminal and in the Code tab of its Desktop window.',
    },
  },
  capa_proyecto: {
    group: 'configuracion',
    es: {
      term: 'Capa de proyecto',
      what: 'El .claude/ y el .mcp.json de un repo.',
      why: 'Para configuración que es del proyecto y viaja con él en git, sea cual sea la cuenta.',
      how: 'La lee Claude Code al trabajar en esa carpeta y gana sobre la de la cuenta. ccp nunca la proyecta ni reescribe; y rechaza un secreto en claro ahí, porque acabaría en git.',
    },
    en: {
      term: 'Project layer',
      what: 'A repo’s .claude/ and .mcp.json.',
      why: 'For configuration that belongs to the project and travels with it in git, whatever the account.',
      how: 'Claude Code reads it while working in that folder and it wins over the account’s. ccp never projects or rewrites it, and it refuses a plain-text secret there, because it would end up in git.',
    },
  },
  capa_chat: {
    group: 'configuracion',
    es: {
      term: 'Chat de Desktop',
      what: 'La configuración del chat de la ventana de Desktop de una cuenta (su claude_desktop_config.json).',
      why: 'Para que el chat de esa ventana tenga los servidores MCP que necesitas.',
      how: 'Solo admite MCP locales (stdio) y los carga al arrancar: con la ventana abierta, hay que reiniciarla. Lo demás (instrucciones, skills…) llega a su pestaña Code desde la capa de la cuenta.',
    },
    en: {
      term: 'Desktop chat',
      what: 'The configuration of the chat in an account’s Desktop window (its claude_desktop_config.json).',
      why: 'So that window’s chat has the MCP servers you need.',
      how: 'It only takes local (stdio) MCP servers and loads them at start-up: with the window open, it needs a restart. The rest (instructions, skills…) reaches its Code tab from the account layer.',
    },
  },
  efectivo: {
    group: 'configuracion',
    es: {
      term: 'Efectivo',
      what: 'Lo que una cuenta recibe de verdad, con todas las capas ya sumadas.',
      why: 'Porque con varias capas no es obvio qué valor gana: esta vista lo dice y de dónde sale.',
      how: 'Cada fila lleva su origen (global, cuenta, sensores…) y tacha lo que otra capa tapa. Para cambiar algo se apaga «Efectivo» y se edita en su capa.',
    },
    en: {
      term: 'Effective',
      what: 'What an account really receives, with every layer already added up.',
      why: 'Because with several layers it isn’t obvious which value wins: this view says so and where it comes from.',
      how: 'Each row carries its origin (global, account, sensors…) and strikes through what another layer overrides. To change something, turn “Effective” off and edit it in its layer.',
    },
  },
  mcp: {
    group: 'configuracion',
    es: {
      term: 'Servidor MCP',
      what: 'Un programa o servicio que le da herramientas nuevas a Claude: leer Figma, consultar una base de datos, buscar en tus documentos…',
      why: 'Para que Claude pueda hacer cosas fuera de la conversación con tus herramientas.',
      how: 'Puede ser local (stdio: un programa que se arranca) o remoto (una URL). Se declara en una capa y sus destinos dicen si llega a Claude Code, al chat de Desktop o a los dos.',
    },
    en: {
      term: 'MCP server',
      what: 'A program or service that gives Claude new tools: reading Figma, querying a database, searching your documents…',
      why: 'So Claude can do things outside the conversation with your tools.',
      how: 'It can be local (stdio: a program that gets started) or remote (a URL). It’s declared in a layer and its targets say whether it reaches Claude Code, the Desktop chat or both.',
    },
  },
  destinos: {
    group: 'configuracion',
    es: {
      term: 'Destinos de un MCP',
      what: 'A dónde llega un servidor MCP: a Claude Code (terminal y pestaña Code), al chat de Desktop, o a los dos.',
      why: 'Para no cargar en el chat lo que solo sirve en la terminal, o al revés.',
      how: 'Van por nombre, no por cuenta: el mismo nombre en dos cuentas comparte destinos. Por defecto, los dos.',
    },
    en: {
      term: 'MCP targets',
      what: 'Where an MCP server goes: to Claude Code (terminal and Code tab), to the Desktop chat, or both.',
      why: 'So you don’t load into the chat what only helps in the terminal, or the other way round.',
      how: 'They go by name, not by account: the same name in two accounts shares targets. Both by default.',
    },
  },
  proyeccion: {
    group: 'configuracion',
    es: {
      term: 'Proyección',
      what: 'La copia que ccp escribe de lo que declaras en una capa, en los archivos que leen Claude Code y Desktop.',
      why: 'Tú declaras en un solo sitio; ccp mantiene al día los varios archivos que cada app lee a su manera.',
      how: 'Ocurre en cada regeneración (al guardar, al sincronizar). ccp solo toca lo que escribió él: lo que pusiste a mano en esos archivos se respeta y se avisa como conflicto.',
    },
    en: {
      term: 'Projection',
      what: 'The copy ccp writes of what you declare in a layer into the files Claude Code and Desktop read.',
      why: 'You declare in one place; ccp keeps up to date the several files each app reads its own way.',
      how: 'It happens on every regeneration (on save, on sync). ccp only touches what it wrote itself: what you put in those files by hand is respected and reported as a conflict.',
    },
  },
  memoria: {
    group: 'configuracion',
    es: {
      term: 'Memoria de Claude',
      what: 'Las instrucciones y artefactos que ccp añadió a la configuración por ti (con /ccp:remember o desde aquí), por alcance: global, cuenta o proyecto.',
      why: 'Para enseñarle cosas a Claude una vez y que las recuerde siempre en ese alcance.',
      how: 'Solo aparece lo que creó ccp; lo que escribiste a mano en esos archivos no se toca.',
    },
    en: {
      term: 'Claude’s memory',
      what: 'The instructions and artifacts ccp added to the configuration for you (with /ccp:remember or from here), by scope: global, account or project.',
      why: 'To teach Claude something once and have it remember it in that scope.',
      how: 'Only what ccp created shows up; what you wrote by hand in those files is left alone.',
    },
  },

  // --- Desktop ------------------------------------------------------------------
  desktop: {
    group: 'desktop',
    es: {
      term: 'Ventana de Desktop',
      what: 'Una ventana de Claude Desktop propia de una cuenta, con su sesión, su chat y su pestaña Code.',
      why: 'Para tener Claude Desktop con varias cuentas abiertas a la vez, cada una en su ventana.',
      how: 'ccp la abre aislada: su propio directorio de datos y el entorno de su cuenta, con el actualizador apagado para que no toque tu Claude principal. Solo cuentas de Anthropic y default.',
    },
    en: {
      term: 'Desktop window',
      what: 'A Claude Desktop window of its own for an account, with its session, its chat and its Code tab.',
      why: 'To have Claude Desktop with several accounts open at once, each in its window.',
      how: 'ccp opens it isolated: its own data directory and its account’s environment, with the updater off so it doesn’t touch your main Claude. Anthropic accounts and default only.',
    },
  },
  lanzador: {
    group: 'desktop',
    es: {
      term: 'Lanzador',
      what: 'La app «Claude (cuenta)» que ccp crea en ~/Applications, con su nombre y su color.',
      why: 'Para distinguir cada ventana en el Dock, Cmd-Tab y Spotlight, y abrirla con un clic.',
      how: 'Envuelve una copia de Claude.app sin modificar. Se reconstruye sola cuando Claude o ccp se actualizan, pero solo con su ventana cerrada.',
    },
    en: {
      term: 'Launcher',
      what: 'The “Claude (account)” app ccp creates in ~/Applications, with its name and color.',
      why: 'To tell each window apart in the Dock, Cmd-Tab and Spotlight, and open it with one click.',
      how: 'It wraps an unmodified copy of Claude.app. It rebuilds itself when Claude or ccp update, but only with its window closed.',
    },
  },
  identidad: {
    group: 'desktop',
    es: {
      term: 'Identidad de la ventana',
      what: 'Que macOS reconozca la ventana como la de su cuenta y no como tu Claude principal.',
      why: 'Si se pierde, abrir tu Claude normal puede traer al frente la ventana de otra cuenta.',
      how: 'Puede perderse si la ventana se reinicia por dentro. El doctor lo detecta y lo dice; cerrarla y abrirla desde su lanzador la recupera. Tus sesiones no se borran.',
    },
    en: {
      term: 'Window identity',
      what: 'macOS recognizing the window as its account’s and not as your main Claude.',
      why: 'If it’s lost, opening your normal Claude can bring another account’s window to the front.',
      how: 'It can be lost if the window restarts itself. The doctor detects and reports it; closing it and opening it from its launcher recovers it. Your sessions aren’t deleted.',
    },
  },
  reiniciar_ventana: {
    group: 'desktop',
    es: {
      term: 'Reiniciar la ventana',
      what: 'Cerrar la ventana de una cuenta y volver a abrirla.',
      why: 'Hay cambios que una ventana abierta no relee (los MCP del chat) y un lanzador no se reconstruye mientras su ventana corre.',
      how: 'ccp la cierra con cuidado y la reabre. Si no se cierra en 20 s, se detiene y lo dice: nunca la mata a la fuerza, porque eso puede dañar su sesión.',
    },
    en: {
      term: 'Restart the window',
      what: 'Closing an account’s window and opening it again.',
      why: 'Some changes aren’t re-read by an open window (the chat’s MCP servers) and a launcher isn’t rebuilt while its window runs.',
      how: 'ccp closes it carefully and reopens it. If it doesn’t close within 20 s, it stops and says so: it never kills it by force, because that can damage its session.',
    },
  },

  // --- Historial y nube --------------------------------------------------------------
  snapshot: {
    group: 'historial',
    es: {
      term: 'Snapshot',
      what: 'Una foto de toda tu configuración en un momento: cuentas, reglas, capas, lo que Claude lee.',
      why: 'Para volver atrás si algo se rompe, ver qué cambió desde entonces o llevártelo a otra máquina.',
      how: 'Se crean a mano, antes de cambios delicados y una vez al día. Restaurar enseña antes el plan y hace otro snapshot primero, por si hay que deshacer.',
    },
    en: {
      term: 'Snapshot',
      what: 'A picture of your whole configuration at one moment: accounts, rules, layers, what Claude reads.',
      why: 'To go back if something breaks, see what changed since, or take it to another machine.',
      how: 'They’re made by hand, before risky changes and once a day. Restoring shows the plan first and takes another snapshot before, in case you need to undo.',
    },
  },
  copia: {
    group: 'historial',
    es: {
      term: 'Copia de seguridad',
      what: 'Un archivo .tar.gz con tu configuración de ccp, con o sin secretos.',
      why: 'Es el formato antiguo, útil para mover a mano una configuración a otra máquina.',
      how: 'Se exporta a un archivo y se restaura viendo antes qué trae. Para el día a día, mejor los snapshots.',
    },
    en: {
      term: 'Backup',
      what: 'A .tar.gz file with your ccp configuration, with or without secrets.',
      why: 'It’s the older format, handy for moving a configuration to another machine by hand.',
      how: 'Export it to a file and restore it seeing first what it brings. For day to day, snapshots are better.',
    },
  },
  nube: {
    group: 'historial',
    es: {
      term: 'Nube',
      what: 'Un servidor tuyo que guarda el historial de snapshots, cifrado de extremo a extremo, y lo reparte entre tus equipos.',
      why: 'Para tener la misma configuración en todas tus máquinas y una copia fuera de esta.',
      how: 'El servidor nunca puede leer lo que guarda. El portal web propone cambios y cada máquina decide si los aplica.',
    },
    en: {
      term: 'Cloud',
      what: 'A server of yours that stores the snapshot history, end-to-end encrypted, and shares it across your devices.',
      why: 'To have the same configuration on all your machines and a copy off this one.',
      how: 'The server can never read what it stores. The web portal proposes changes and each machine decides whether to apply them.',
    },
  },
  boveda: {
    group: 'historial',
    es: {
      term: 'Bóveda',
      what: 'La clave que cifra todo lo que sube a la nube, protegida por tu frase de bóveda y un código de recuperación.',
      why: 'Para que ni el servidor ni nadie con acceso a él pueda leer tu configuración.',
      how: 'Se crea una vez y se desbloquea en cada equipo con la frase. La frase nunca pasa por esta app: se escribe en una terminal.',
    },
    en: {
      term: 'Vault',
      what: 'The key that encrypts everything uploaded to the cloud, protected by your vault passphrase and a recovery code.',
      why: 'So neither the server nor anyone with access to it can read your configuration.',
      how: 'It’s created once and unlocked on each device with the passphrase. The passphrase never goes through this app: it’s typed in a terminal.',
    },
  },
  sincronizar: {
    group: 'historial',
    es: {
      term: 'Sincronizar',
      what: 'Guardar un snapshot si cambió algo y subir a la nube todo lo que falte.',
      why: 'Para que lo que tienes ahora quede arriba, sin tener que crear el snapshot a mano.',
      how: 'Solo sube lo que la nube no tiene. Si no cambió nada, lo dice en vez de subir otra vez.',
    },
    en: {
      term: 'Sync',
      what: 'Saving a snapshot if anything changed and uploading everything the cloud doesn’t have yet.',
      why: 'So what you have right now ends up in the cloud, without making the snapshot by hand.',
      how: 'It only uploads what the cloud lacks. If nothing changed, it says so instead of uploading again.',
    },
  },
  revision: {
    group: 'historial',
    es: {
      term: 'Revisión pendiente',
      what: 'Un cambio que llegó del portal y que esta máquina no aplicó sola porque ejecuta código (hooks, MCP, barra de estado…).',
      why: 'Es la barrera de seguridad: lo que puede ejecutar algo en tu máquina lo confirma alguien que está en ella.',
      how: 'Se aprueba elemento a elemento, nada viene marcado, y lo que no marques se rechaza y se informa al portal.',
    },
    en: {
      term: 'Pending review',
      what: 'A change from the portal that this machine didn’t apply on its own because it runs code (hooks, MCP, status line…).',
      why: 'It’s the safety barrier: whatever can execute something on your machine is confirmed by someone at it.',
      how: 'Approve item by item, nothing comes pre-checked, and whatever you leave unchecked is rejected and reported to the portal.',
    },
  },

  // --- La app -------------------------------------------------------------------------
  diagnostico: {
    group: 'app',
    es: {
      term: 'Diagnóstico',
      what: 'La lista de lo que está mal o a medias: cuentas sin login, sensores que faltan, cadenas que no pueden rotar, ventanas sin identidad…',
      why: 'Para enterarte antes de que un fallo te pille trabajando.',
      how: 'Explica qué significa cada aviso y cómo se arregla, con un botón cuando se puede. No repara nada por su cuenta. Lo que no pudo comprobar lo marca como «desconocido», nunca como correcto.',
    },
    en: {
      term: 'Diagnostics',
      what: 'The list of what’s wrong or half-done: accounts without login, missing sensors, chains that can’t rotate, windows without identity…',
      why: 'To find out before a failure catches you mid-work.',
      how: 'It explains what each warning means and how to fix it, with a button when possible. It repairs nothing on its own. What it couldn’t check is marked “unknown”, never OK.',
    },
  },
  cli: {
    group: 'app',
    es: {
      term: 'Equivalente CLI',
      what: 'El comando de terminal que hace lo mismo que la pantalla o el botón.',
      why: 'Todo lo que hace la app se puede hacer desde la terminal: para scripts, para aprender o para cuando no tengas la app a mano.',
      how: 'Se copia con un clic. La app usa el mismo motor que ese comando, así que el resultado es idéntico.',
    },
    en: {
      term: 'CLI equivalent',
      what: 'The terminal command that does the same as the screen or button.',
      why: 'Everything the app does can be done from the terminal: for scripts, to learn, or when you don’t have the app at hand.',
      how: 'Copy it with one click. The app uses the same engine as that command, so the result is identical.',
    },
  },
};

export function glossary(key: string): GlossaryText | undefined {
  const e = G[key];
  if (!e) return undefined;
  return getLang() === 'en' ? e.en : e.es;
}

export function glossaryKeys(): string[] {
  return Object.keys(G);
}

export function glossaryGroup(key: string): GlossaryGroup | undefined {
  return G[key]?.group;
}

export const GLOSSARY_GROUPS: GlossaryGroup[] = ['cuentas', 'conversaciones', 'rotacion', 'configuracion', 'desktop', 'historial', 'app'];
