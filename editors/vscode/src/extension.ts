import * as vscode from "vscode";
import {
    LanguageClient,
    type LanguageClientOptions,
    type ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;
let output: vscode.LogOutputChannel;

function serverOptions(): { command: string; args: string[] } {
    const config = vscode.workspace.getConfiguration("kigumi");
    const command = config.get<string>("serverPath") || "kigumi";
    const stdRoot = config.get<string>("stdRoot") || "";
    const args = ["lsp"];
    if (stdRoot !== "") {
        args.push("--std", stdRoot);
    }
    return { command, args };
}

async function start(context: vscode.ExtensionContext): Promise<void> {
    const config = vscode.workspace.getConfiguration("kigumi");
    const server = serverOptions();
    const clientOptions: LanguageClientOptions = {
        documentSelector: [{ scheme: "file", language: "kigumi" }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher("**/*.kg"),
        },
        initializationOptions: { stdRoot: config.get<string>("stdRoot") || "" },
        outputChannel: output,
        traceOutputChannel: output,
    };
    client = new LanguageClient(
        "kigumi",
        "Kigumi Language Server",
        server as ServerOptions,
        clientOptions,
    );
    context.subscriptions.push(client);
    output.info(`starting ${server.command} ${server.args.join(" ")}`);
    try {
        await client.start();
    } catch (err) {
        output.error(`could not start ${server.command}: ${err}`);
        output.show(true);
        void vscode.window.showErrorMessage(
            `Kigumi: could not start "${server.command} lsp". Build it with "go install ." in the repository or set kigumi.serverPath.`,
        );
    }
}

async function stop(): Promise<void> {
    if (client) {
        output.info("stopping language server");
        await client.stop();
        client = undefined;
    }
}

export async function activate(
    context: vscode.ExtensionContext,
): Promise<void> {
    output = vscode.window.createOutputChannel("Kigumi Language Server", {
        log: true,
    });
    context.subscriptions.push(
        output,
        vscode.commands.registerCommand("kigumi.showServerLog", () =>
            output.show(),
        ),
        vscode.commands.registerCommand("kigumi.restartServer", async () => {
            await stop();
            await start(context);
        }),
        vscode.workspace.onDidChangeConfiguration(async (e) => {
            if (
                e.affectsConfiguration("kigumi.serverPath") ||
                e.affectsConfiguration("kigumi.stdRoot")
            ) {
                output.info("configuration changed, restarting");
                await vscode.commands.executeCommand("kigumi.restartServer");
            }
        }),
    );
    await start(context);
}

export async function deactivate(): Promise<void> {
    await stop();
}
