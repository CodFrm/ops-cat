export namespace ai {
	
	export class CLIInfo {
	    name: string;
	    type: string;
	    path: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new CLIInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.path = source["path"];
	        this.version = source["version"];
	    }
	}
	export class ToolCall {
	    id: string;
	    // Go type: struct { Name string "json:\"name\""; Arguments string "json:\"arguments\"" }
	    function: any;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.function = this.convertValues(source["function"], Object);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Message {
	    role: string;
	    content: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace asset_entity {
	
	export class Asset {
	    ID: number;
	    Name: string;
	    Type: string;
	    GroupID: number;
	    Icon: string;
	    Tags: string;
	    Description: string;
	    Config: string;
	    CmdPolicy: string;
	    SortOrder: number;
	    Status: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Type = source["Type"];
	        this.GroupID = source["GroupID"];
	        this.Icon = source["Icon"];
	        this.Tags = source["Tags"];
	        this.Description = source["Description"];
	        this.Config = source["Config"];
	        this.CmdPolicy = source["CmdPolicy"];
	        this.SortOrder = source["SortOrder"];
	        this.Status = source["Status"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}

}

export namespace audit_entity {
	
	export class AuditLog {
	    ID: number;
	    Source: string;
	    ToolName: string;
	    AssetID: number;
	    AssetName: string;
	    Command: string;
	    Request: string;
	    Result: string;
	    Error: string;
	    Success: number;
	    ConversationID: number;
	    PlanSessionID: string;
	    Createtime: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Source = source["Source"];
	        this.ToolName = source["ToolName"];
	        this.AssetID = source["AssetID"];
	        this.AssetName = source["AssetName"];
	        this.Command = source["Command"];
	        this.Request = source["Request"];
	        this.Result = source["Result"];
	        this.Error = source["Error"];
	        this.Success = source["Success"];
	        this.ConversationID = source["ConversationID"];
	        this.PlanSessionID = source["PlanSessionID"];
	        this.Createtime = source["Createtime"];
	    }
	}

}

export namespace backup_svc {
	
	export class DeviceFlowInfo {
	    deviceCode: string;
	    userCode: string;
	    verificationUri: string;
	    expiresIn: number;
	    interval: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceFlowInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceCode = source["deviceCode"];
	        this.userCode = source["userCode"];
	        this.verificationUri = source["verificationUri"];
	        this.expiresIn = source["expiresIn"];
	        this.interval = source["interval"];
	    }
	}
	export class GistInfo {
	    id: string;
	    description: string;
	    updatedAt: string;
	    htmlUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new GistInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.updatedAt = source["updatedAt"];
	        this.htmlUrl = source["htmlUrl"];
	    }
	}
	export class GitHubUser {
	    login: string;
	    avatarUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.avatarUrl = source["avatarUrl"];
	    }
	}

}

export namespace conversation_entity {
	
	export class ContentBlock {
	    type: string;
	    content: string;
	    toolName?: string;
	    toolInput?: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContentBlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.toolName = source["toolName"];
	        this.toolInput = source["toolInput"];
	        this.status = source["status"];
	    }
	}
	export class Conversation {
	    ID: number;
	    Title: string;
	    ProviderType: string;
	    Model: string;
	    SessionData: string;
	    WorkDir: string;
	    Status: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Title = source["Title"];
	        this.ProviderType = source["ProviderType"];
	        this.Model = source["Model"];
	        this.SessionData = source["SessionData"];
	        this.WorkDir = source["WorkDir"];
	        this.Status = source["Status"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}

}

export namespace group_entity {
	
	export class Group {
	    ID: number;
	    Name: string;
	    ParentID: number;
	    Icon: string;
	    CmdPolicy: string;
	    SortOrder: number;
	    Createtime: number;
	    Updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.ParentID = source["ParentID"];
	        this.Icon = source["Icon"];
	        this.CmdPolicy = source["CmdPolicy"];
	        this.SortOrder = source["SortOrder"];
	        this.Createtime = source["Createtime"];
	        this.Updatetime = source["Updatetime"];
	    }
	}

}

export namespace import_svc {
	
	export class ImportError {
	    name: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.reason = source["reason"];
	    }
	}
	export class ImportResult {
	    total: number;
	    success: number;
	    skipped: number;
	    failed: number;
	    errors: ImportError[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.success = source["success"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.errors = this.convertValues(source["errors"], ImportError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PreviewGroup {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new PreviewGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class PreviewItem {
	    index: number;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authType: string;
	    groupId: string;
	    exists: boolean;
	    hasPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authType = source["authType"];
	        this.groupId = source["groupId"];
	        this.exists = source["exists"];
	        this.hasPassword = source["hasPassword"];
	    }
	}
	export class PreviewResult {
	    groups: PreviewGroup[];
	    items: PreviewItem[];
	    hasVault: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], PreviewGroup);
	        this.items = this.convertValues(source["items"], PreviewItem);
	        this.hasVault = source["hasVault"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AuditLogListResult {
	    items: audit_entity.AuditLog[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditLogListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], audit_entity.AuditLog);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConversationDisplayMessage {
	    role: string;
	    content: string;
	    blocks: conversation_entity.ContentBlock[];
	
	    static createFrom(source: any = {}) {
	        return new ConversationDisplayMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.blocks = this.convertValues(source["blocks"], conversation_entity.ContentBlock);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportFileInfo {
	    filePath: string;
	    encrypted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filePath = source["filePath"];
	        this.encrypted = source["encrypted"];
	    }
	}
	export class LocalSSHKeyInfo {
	    path: string;
	    keyType: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalSSHKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
	    }
	}
	export class OpsctlInfo {
	    installed: boolean;
	    path: string;
	    version: string;
	    embedded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpsctlInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.embedded = source["embedded"];
	    }
	}
	export class SSHConnectRequest {
	    assetId: number;
	    password: string;
	    key: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new SSHConnectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assetId = source["assetId"];
	        this.password = source["password"];
	        this.key = source["key"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class SkillInfo {
	    installed: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.path = source["path"];
	    }
	}

}

export namespace sftp_svc {
	
	export class FileEntry {
	    name: string;
	    size: number;
	    isDir: boolean;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.isDir = source["isDir"];
	        this.modTime = source["modTime"];
	    }
	}

}

export namespace ssh_key_entity {
	
	export class SSHKey {
	    id: number;
	    name: string;
	    comment: string;
	    keyType: string;
	    keySize: number;
	    publicKey: string;
	    fingerprint: string;
	    createtime: number;
	    updatetime: number;
	
	    static createFrom(source: any = {}) {
	        return new SSHKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.comment = source["comment"];
	        this.keyType = source["keyType"];
	        this.keySize = source["keySize"];
	        this.publicKey = source["publicKey"];
	        this.fingerprint = source["fingerprint"];
	        this.createtime = source["createtime"];
	        this.updatetime = source["updatetime"];
	    }
	}

}

export namespace sshpool {
	
	export class PoolEntryInfo {
	    asset_id: number;
	    ref_count: number;
	    last_used: number;
	
	    static createFrom(source: any = {}) {
	        return new PoolEntryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asset_id = source["asset_id"];
	        this.ref_count = source["ref_count"];
	        this.last_used = source["last_used"];
	    }
	}

}

