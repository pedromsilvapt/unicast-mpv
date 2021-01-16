import { Server as WebSocketServer } from 'rpc-websockets';
import { NativeCommands } from './Commands/NativeCommands';
import { Logger, ConsoleBackend, ActivityLogger, Activity, ActivityLoggerHFP } from 'clui-logger';
import { TIMESTAMP_SHORT } from 'clui-logger/lib/Backends/ConsoleBackend';
import { StatusCommand } from './Commands/StatusCommand';
import { QuitCommand } from './Commands/QuitCommand';
import { PlayCommand } from './Commands/PlayCommand';
import chalk from 'chalk';
import { Config } from './Config';
import { Player } from './Player';
import { Events } from './Events';
import path from 'path';

export interface CommandPreHook<I extends any[] = any[]> {
    ( args : I, command : string, ctx : any ) : unknown;
}

export interface CommandPostHook<I extends any[] = any[], O = any> {
    ( args : I, command : string, error : any, result : O, ctx : any ) : unknown;
}

export class UnicastMpv {
    public static baseConfig () : Config {
        return Config.load( path.join( __dirname, '..', 'config' ) );
    }

    public readonly config : Config;

    public readonly logger : Logger;

    public player : Player;

    public connection : any;

    protected eventHooks : Map<string, CommandPreHook[]> = new Map();

    protected preHooks : Map<string, CommandPreHook[]> = new Map();
    
    protected postHooks : Map<string, CommandPostHook[]> = new Map();
    
    protected globalEventHooks : CommandPreHook[] = [];

    protected globalPreHooks : CommandPreHook[] = [];

    protected globalPostHooks : CommandPostHook[] = [];

    constructor ( config ?: Config, logger ?: Logger ) {
        this.config = config || UnicastMpv.baseConfig();

        this.logger = logger || new Logger( new ConsoleBackend( TIMESTAMP_SHORT ) );

        this.player = new Player( this.config.slice( 'player' ) );

        this.player.observeProperty( 'sub-scale' );
    }

    registerEventHook ( event : string, fn : CommandPreHook ) {
        let hooks = this.eventHooks.get( event );

        if ( !hooks ) {
            this.eventHooks.set( event, hooks = [] );
        }

        hooks.push( fn );
    }

    registerPreHook ( command : string, fn : CommandPreHook ) {
        let hooks = this.preHooks.get( command );

        if ( !hooks ) {
            this.preHooks.set( command, hooks = [] );
        }

        hooks.push( fn );
    }

    registerPostHook ( command : string, fn : CommandPostHook ) {
        let hooks = this.postHooks.get( command );

        if ( !hooks ) {
            this.postHooks.set( command, hooks = [] );
        }

        hooks.push( fn );
    }

    registerGlobalEventHook ( fn : CommandPreHook ) {
        this.globalEventHooks.push( fn );
    }

    registerGlobalPreHook ( fn : CommandPreHook ) {
        this.globalPreHooks.push( fn );
    }

    registerGlobalPostHook ( fn : CommandPostHook ) {
        this.globalPostHooks.push( fn );
    }

    protected async triggerPreHooks ( command : string, args : any[], ctx : any ) {
        for ( let hook of this.globalPreHooks ) {
            await hook( args, command, ctx );
        }
        
        const preHooks : CommandPreHook[] = this.preHooks.get( command );

        if ( preHooks != null ) {
            for ( let hook of preHooks ) {
                await hook( args, command, ctx );
            }
        }
    }

    protected async triggerPostHooks ( command : string, args : any[], error : any, result : any, ctx : any ) {
        for ( let hook of this.globalPostHooks ) {
            await hook( args, command, error, result, ctx );
        }

        const postHooks : CommandPostHook[] = this.postHooks.get( command );

        if ( postHooks != null ) {
            for ( let hook of postHooks ) {
                await hook( args, command, error, result, ctx );
            }
        }
    }

    protected async triggerEventHooks ( event : string, args : any[], ctx : any ) {
        for ( let hook of this.globalEventHooks ) {
            await hook( args, event, ctx );
        }
        
        const eventHooks : CommandPreHook[] = this.eventHooks.get( event );

        if ( eventHooks != null ) {
            for ( let hook of eventHooks ) {
                await hook( args, event, ctx );
            }
        }
    }

    event ( name : string ) {
        this.connection.event( name );
    }

    async emit ( event : string, ...args : any[] ) {
        const ctx = {};

        await this.triggerEventHooks( event, args, ctx );
        
        this.connection.emit( event, ...args );
    }

    register ( command : string, fn : Function ) {
        this.connection.register( command, async ( args : any[] ) => {
            const ctx = {};

            await this.triggerPreHooks( command, args, ctx );

            try {
                const result = await fn( args );

                await this.triggerPostHooks( command, args, null, result, ctx );

                return result;
            } catch ( error ) {
                await this.triggerPostHooks( command, args, error, null, ctx );
            }
        } );
    }

    async listen () : Promise<void> {
        this.connection = new WebSocketServer({
            port: this.config.get( 'server.port' ),
            host: this.config.get( 'server.address' )
        } );

        const rpcLogger = this.logger.service( 'rpc' );

        // Ignore events when logging them to the console
        const ignoredEvents = [ 'status' ];

        const activityLogger = new RpcActivityLogger( rpcLogger );

        activityLogger.registerHighFrequencyPattern( /status/, null, 60 * 5 );

        this.registerGlobalPreHook( activityLogger.before() );
        this.registerGlobalPostHook( activityLogger.after() );
        this.registerGlobalEventHook( ( args, event, ctx ) => {
            if ( !ignoredEvents.includes( event ) ) {
                rpcLogger.service( event ).debug( chalk.cyan( 'emit ' ) + JSON.stringify( args ) );
            }
        } );

        new NativeCommands( this );
        new StatusCommand( this );
        new QuitCommand( this );
        new PlayCommand( this );
        new StatusCommand( this );

        new Events( this );

        await new Promise( ( resolve, reject ) => {
            this.connection.on( 'listening', resolve )
    
            this.connection.on( 'error', reject );
        } );

        this.logger.info( 'Server listening on port ' + this.config.get( 'server.port' ) );
    }
}

export interface CommandActivity extends Activity {
    command: string;
    args: any[];
    error ?: any;
    result ?: any;
}

export class RpcActivityLogger extends ActivityLogger<CommandActivity> {
    // The names of commands to ignore when loggin (as to not pollute the console output). Errored commands are always logged
    public ignoredCommands = [];
    // Log ignored commands if they take more than time this to complete (in milliseconds)
    public ignoredCommandMaxTime = 300;

    protected findHighFrequencyPattern ( activity: CommandActivity ) : ActivityLoggerHFP {
        return this.highFrequencyPatterns.find( pattern => pattern.pattern.test( activity.command ) );
    }

    protected matchHighFrequencyPattern ( pattern: ActivityLoggerHFP, activity: CommandActivity ) : string[] {
        return activity.command.match( pattern.pattern );
    }

    protected logBeginActivity ( activity: CommandActivity ) : void {
        const { command, args, live } = activity;

        if ( !this.ignoredCommands.includes( command ) ) {
            live.service( command ).debug( chalk.grey( `${ args.join( ' ' ) } running...` ) );
        }
    }

    protected logEndActivity ( activity: CommandActivity ) : void {
        const { command, args, error, stopwatch, live } = activity;

        const forceLogCommand = error || stopwatch.readMilliseconds() > this.ignoredCommandMaxTime;

        const logger = live.service( command );

        if ( !this.ignoredCommands.includes( command ) || forceLogCommand ) {
            logger.update( () => {
                logger.debug( `${ args.join( ' ' ) } ${ stopwatch.readHumanized() } ${ error ? chalk.red( 'FAILED' ) : '' }` );

                if ( error && error.message ) {
                    logger.error( error.message + ( error.stack ? ( '\n' + error.stack ) : '' ), error );
                } else if ( error && error.errcode && error.errmessage ) {
                    logger.error( `CODE ${ error.errcode } ${ error.method }: ${ error.errmessage }`, error );
                }
            } );
        }
    }

    public before () {
        return ( args : any[], command : string, ctx : CommandActivity ) => {
            ctx.command = command;
            ctx.args = args;
            
            this.begin( ctx );
        };
    }

    public after () {
        return ( args : any[], command : string, error : any, result : any, ctx : CommandActivity ) => {
            ctx.error = error;
            ctx.result = result;

            this.end( ctx );
        };
    }
}
