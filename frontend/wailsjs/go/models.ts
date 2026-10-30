export namespace copier {
	
	export class Options {
	    noOwner: boolean;
	    noAcl: boolean;
	    exitOnError: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.noOwner = source["noOwner"];
	        this.noAcl = source["noAcl"];
	        this.exitOnError = source["exitOnError"];
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

