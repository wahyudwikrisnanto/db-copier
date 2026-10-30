export namespace copier {
	
	export class ActiveTable {
	    name: string;
	    bytes: number;
	    done: number;
	
	    static createFrom(source: any = {}) {
	        return new ActiveTable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	        this.done = source["done"];
	    }
	}
	export class Options {
	    noOwner: boolean;
	    noAcl: boolean;
	    exitOnError: boolean;
	    jobs: number;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.noOwner = source["noOwner"];
	        this.noAcl = source["noAcl"];
	        this.exitOnError = source["exitOnError"];
	        this.jobs = source["jobs"];
	    }
	}
	export class Progress {
	    phase: string;
	    parallel: boolean;
	    tablesDone: number;
	    tablesTotal: number;
	    bytesDone: number;
	    bytesTotal: number;
	    streamed: number;
	    active: ActiveTable[];
	    schemaObjects: number;
	    objectsDone: number;
	    objectsTotal: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.parallel = source["parallel"];
	        this.tablesDone = source["tablesDone"];
	        this.tablesTotal = source["tablesTotal"];
	        this.bytesDone = source["bytesDone"];
	        this.bytesTotal = source["bytesTotal"];
	        this.streamed = source["streamed"];
	        this.active = this.convertValues(source["active"], ActiveTable);
	        this.schemaObjects = source["schemaObjects"];
	        this.objectsDone = source["objectsDone"];
	        this.objectsTotal = source["objectsTotal"];
	        this.errors = source["errors"];
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

export namespace history {
	
	export class ConnRef {
	    id: string;
	    name: string;
	    color: string;
	    host: string;
	    port: number;
	    database: string;
	    user: string;
	    sshHost?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.user = source["user"];
	        this.sshHost = source["sshHost"];
	    }
	}
	export class CopyDetail {
	    plan?: pg.Plan;
	    progress?: copier.Progress;
	    doneTables: string[];
	
	    static createFrom(source: any = {}) {
	        return new CopyDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan = this.convertValues(source["plan"], pg.Plan);
	        this.progress = this.convertValues(source["progress"], copier.Progress);
	        this.doneTables = source["doneTables"];
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
	export class CopyInfo {
	    options: copier.Options;
	    tablesTotal: number;
	    tablesDone: number;
	    bytesTotal: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new CopyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.options = this.convertValues(source["options"], copier.Options);
	        this.tablesTotal = source["tablesTotal"];
	        this.tablesDone = source["tablesDone"];
	        this.bytesTotal = source["bytesTotal"];
	        this.errors = source["errors"];
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
	export class Detail {
	    id: string;
	    kind: string;
	    status: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	    durationMs: number;
	    error?: string;
	    source?: ConnRef;
	    target?: ConnRef;
	    copy?: CopyInfo;
	    copyDetail?: CopyDetail;
	    log: string[];
	    logTruncated: boolean;
	    logPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Detail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.source = this.convertValues(source["source"], ConnRef);
	        this.target = this.convertValues(source["target"], ConnRef);
	        this.copy = this.convertValues(source["copy"], CopyInfo);
	        this.copyDetail = this.convertValues(source["copyDetail"], CopyDetail);
	        this.log = source["log"];
	        this.logTruncated = source["logTruncated"];
	        this.logPath = source["logPath"];
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
	export class Job {
	    id: string;
	    kind: string;
	    status: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    finishedAt?: any;
	    durationMs: number;
	    error?: string;
	    source?: ConnRef;
	    target?: ConnRef;
	    copy?: CopyInfo;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.source = this.convertValues(source["source"], ConnRef);
	        this.target = this.convertValues(source["target"], ConnRef);
	        this.copy = this.convertValues(source["copy"], CopyInfo);
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
	
	export class ToolInfo {
	    path: string;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}
	export class SettingsInfo {
	    settings: store.Settings;
	    pgDump: ToolInfo;
	    pgRestore: ToolInfo;
	    configDir: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], store.Settings);
	        this.pgDump = this.convertValues(source["pgDump"], ToolInfo);
	        this.pgRestore = this.convertValues(source["pgRestore"], ToolInfo);
	        this.configDir = source["configDir"];
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

export namespace pg {
	
	export class Table {
	    name: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Table(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes = source["bytes"];
	    }
	}
	export class Plan {
	    tables: Table[];
	    postData: number;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tables = this.convertValues(source["tables"], Table);
	        this.postData = source["postData"];
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

export namespace store {
	
	export class SSHConfig {
	    enabled: boolean;
	    host: string;
	    port: number;
	    user: string;
	    authMethod: string;
	    keyPath: string;
	    password?: string;
	    passphrase?: string;
	
	    static createFrom(source: any = {}) {
	        return new SSHConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	        this.password = source["password"];
	        this.passphrase = source["passphrase"];
	    }
	}
	export class Connection {
	    id: string;
	    name: string;
	    color: string;
	    host: string;
	    port: number;
	    database: string;
	    user: string;
	    password?: string;
	    sslMode: string;
	    ssh: SSHConfig;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.sslMode = source["sslMode"];
	        this.ssh = this.convertValues(source["ssh"], SSHConfig);
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
	
	export class Settings {
	    pgDumpPath: string;
	    pgRestorePath: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pgDumpPath = source["pgDumpPath"];
	        this.pgRestorePath = source["pgRestorePath"];
	    }
	}

}

