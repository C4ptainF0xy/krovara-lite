import { client, registerPresenceParser, xml } from './client';
import { snapshot } from '$lib/stores/auth';

const NODE = 'urn:xmpp:user-status';
const NS_PUBSUB = 'http://jabber.org/protocol/pubsub';
const NS_PUBSUB_EVENT = 'http://jabber.org/protocol/pubsub#event';
const KROVARA_NS = 'urn:krovara:presence:1';

export type RichPresence = {
  game: string;
  details?: string;
  state?: string;
  since?: number;
  iconUrl?: string;
};

export async function publishPresence(p: RichPresence | null): Promise<void> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  const itemChildren = [];
  if (p) {
    const payload = xml('presence', { xmlns: KROVARA_NS });
    set(payload, 'game', p.game);
    if (p.details) set(payload, 'details', p.details);
    if (p.state) set(payload, 'state', p.state);
    if (p.since) set(payload, 'since', String(p.since));
    if (p.iconUrl) set(payload, 'icon_url', p.iconUrl);
    itemChildren.push(payload);
  }
  await c.send(
    xml(
      'iq',
      { type: 'set', id: 'presence-' + Date.now() },
      xml(
        'pubsub',
        { xmlns: NS_PUBSUB },
        xml(
          'publish',
          { node: NODE },
          xml('item', { id: 'current' }, ...itemChildren)
        )
      )
    )
  );
}

export function clearPresence(): Promise<void> {
  return publishPresence(null);
}

function set(parent: ReturnType<typeof xml>, name: string, value: string): void {
  (parent as unknown as { children: unknown[] }).children.push(
    xml(name, {}, value) as unknown
  );
}

type PresenceListener = (fromBareJID: string, p: RichPresence | null) => void;
const listeners = new Set<PresenceListener>();

export function onPresence(fn: PresenceListener): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

type XmlNode = {
  attrs: Record<string, string>;
  getChild: (n: string, ns?: string) => XmlNode | undefined;
  getChildText: (n: string) => string | undefined;
  children?: unknown[];
};

export function parsePresenceMessage(stanza: XmlNode): boolean {
  const event = stanza.getChild('event', NS_PUBSUB_EVENT);
  if (!event) return false;
  const items = event.getChild('items');
  if (!items || items.attrs?.node !== NODE) return false;
  const fromFull = stanza.attrs?.from ?? '';
  const bare = fromFull.split('/')[0] ?? fromFull;

  const item = items.getChild('item');
  const payload = item?.getChild('presence', KROVARA_NS);
  if (!payload) {
    listeners.forEach((fn) => fn(bare, null));
    return true;
  }
  const game = payload.getChildText('game') ?? '';
  const details = payload.getChildText('details');
  const state = payload.getChildText('state');
  const since = payload.getChildText('since');
  const iconUrl = payload.getChildText('icon_url');
  if (!game) {
    listeners.forEach((fn) => fn(bare, null));
    return true;
  }
  listeners.forEach((fn) =>
    fn(bare, {
      game,
      details: details || undefined,
      state: state || undefined,
      since: since ? Number(since) : undefined,
      iconUrl: iconUrl || undefined
    })
  );
  return true;
}

registerPresenceParser(parsePresenceMessage);

export const _auth = snapshot;
