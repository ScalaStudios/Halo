import type { RemoteSession } from "@/lib/infra-types";

export async function remoteDesktop(host: HTMLElement) {
  const { default: Guacamole } = await import("@pelotech/guacamole-common-js");
  return (socket: WebSocket, session: RemoteSession) => {
    class DesktopTunnel extends Guacamole.Tunnel {}
    const tunnel = new DesktopTunnel();
    const send = (opcode: string, ...args: unknown[]) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "desktop", opcode, args: args.map(String) }));
    };
    tunnel.sendMessage = send;
    tunnel.connect = () => {
      tunnel.state = Guacamole.Tunnel.State.OPEN;
      tunnel.onstatechange?.(tunnel.state);
    };
    tunnel.disconnect = () => {
      tunnel.state = Guacamole.Tunnel.State.CLOSED;
      tunnel.onstatechange?.(tunnel.state);
    };
    const client = new Guacamole.Client(tunnel);
    const parser = new Guacamole.Parser();
    const decoder = new TextDecoder();
    parser.oninstruction = (opcode, args) => tunnel.oninstruction?.(opcode, args);
    const display = client.getDisplay();
    const element = display.getElement();
    element.tabIndex = 0;
    element.setAttribute("aria-label", `${session.protocol.toUpperCase()} remote desktop. Press Shift and Escape to return to Halo controls.`);
    const escape = (event: KeyboardEvent) => {
      if (event.shiftKey && event.key === "Escape") {
        event.preventDefault();
        event.stopImmediatePropagation();
        element.blur();
      }
    };
    element.addEventListener("keydown", escape, { capture: true });
    const keyboard = new Guacamole.Keyboard(element);
    keyboard.onkeydown = (key) => { client.sendKeyEvent(1, key); };
    keyboard.onkeyup = (key) => client.sendKeyEvent(0, key);
    element.addEventListener("blur", keyboard.reset);
    const mouse = new Guacamole.Mouse(element);
    mouse.onEach(["mousedown", "mousemove", "mouseup"], (event) => {
      client.sendMouseState((event as Guacamole.Mouse.Event).state, true);
    });
    const touchscreen = new Guacamole.Mouse.Touchscreen(element);
    touchscreen.onEach(["mousedown", "mousemove", "mouseup"], (event) => {
      client.sendMouseState((event as Guacamole.Mouse.Event).state, true);
    });
    host.replaceChildren(element);
    const fit = () => {
      const width = display.getWidth();
      const height = display.getHeight();
      if (width && height) display.scale(Math.min(1, host.clientWidth / width, host.clientHeight / height));
    };
    display.onresize = fit;
    const observer = new ResizeObserver(fit);
    observer.observe(host);
    client.connect();
    element.focus();
    return {
      receive: (data: ArrayBuffer) => parser.receive(decoder.decode(data, { stream: true })),
      dispose: () => {
        keyboard.reset();
        observer.disconnect();
        client.disconnect();
        element.remove();
      },
    };
  };
}
