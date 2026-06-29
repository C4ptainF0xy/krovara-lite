import { client, xml, boundNickname } from './client';

const MAM_NS = 'urn:xmpp:mam:2';
const RSM_NS = 'http://jabber.org/protocol/rsm';
const DATA_NS = 'jabber:x:data';

function bareDomain(): string {
  const c = client();
  return c ? ((c.options as { domain?: string }).domain ?? '') : '';
}

export function userJID(userUuid: string): string {
  return `${userUuid}@${bareDomain()}`;
}

export function myUuid(): string {
  return boundNickname() ?? '';
}

let dmSeq = 0;
function messageID(): string {
  return `dm-${boundNickname() ?? 'x'}-${++dmSeq}-${performance.now().toString(36)}`;
}

export async function sendDirectMessage(
  peerUuid: string,
  body: string,
  opts?: { oobs?: { url: string; name?: string }[]; replaceId?: string }
): Promise<string> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  const id = messageID();

  const children = [
    xml('body', {}, body),
    xml('origin-id', { xmlns: 'urn:xmpp:sid:0', id }),
  ];

  if (opts?.replaceId) {
    children.push(xml('replace', { xmlns: 'urn:xmpp:message-correct:0', id: opts.replaceId }));
  }

  if (opts?.oobs) {
    for (const oob of opts.oobs) {
      children.push(
        xml('x', { xmlns: 'jabber:x:oob' }, xml('url', {}, oob.url), xml('desc', {}, oob.name || ''))
      );
    }
  }

  const msg = xml('message', { to: userJID(peerUuid), type: 'chat', id }, ...children);

  await c.send(msg);
  return id;
}

export async function retractDirectMessage(peerUuid: string, originId: string): Promise<void> {
  const c = client();
  if (!c) throw new Error('xmpp not connected');
  const id = messageID();
  const msg = xml(
    'message',
    { to: userJID(peerUuid), type: 'chat', id },
    xml('retract', { xmlns: 'urn:xmpp:message-retract:0', id: originId }),
    xml('fallback', { xmlns: 'urn:xmpp:fallback:0' }),
    xml('body', {}, 'Ce message a été supprimé.')
  );
  await c.send(msg);
}

export async function fetchDmHistory(peerUuid: string, count = 50): Promise<void> {
  const c = client();
  if (!c) return;
  await c.send(
    xml(
      'iq',
      { type: 'set', id: 'dmmam-' + Date.now() },
      xml(
        'query',
        { xmlns: MAM_NS, queryid: 'dmmam-' + peerUuid },
        xml(
          'x',
          { xmlns: DATA_NS, type: 'submit' },
          xml('field', { var: 'FORM_TYPE', type: 'hidden' }, xml('value', {}, MAM_NS)),
          xml('field', { var: 'with' }, xml('value', {}, userJID(peerUuid)))
        ),
        xml('set', { xmlns: RSM_NS }, xml('max', {}, String(count)), xml('before', {}))
      )
    )
  );
}
