declare module '@xmpp/client/browser' {
  export type XMLElement = {
    is: (name: string, ns?: string) => boolean;
    attrs: Record<string, string>;
    children: unknown[];
    getChild: (name: string, ns?: string) => XMLElement | undefined;
    getChildText: (name: string) => string | undefined;
    toString: () => string;
  };

  export type XMPPClient = {
    options: { domain?: string };
    on(event: 'online' | 'offline' | 'error' | 'stanza', listener: (arg: unknown) => void): void;
    start(): Promise<unknown>;
    stop(): Promise<void>;
    send(stanza: XMLElement): Promise<void>;
  };

  export function client(opts: {
    service: string;
    domain: string;
    username: string;
    password: string;
    resource?: string;
  }): XMPPClient;

  export function xml(
    name: string,
    attrs?: Record<string, string>,
    ...children: Array<XMLElement | string | undefined>
  ): XMLElement;
}
