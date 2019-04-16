import { Server as WebSocketServer } from 'rpc-websockets';
import { NativeCommands } from './Commands/NativeCommands';
import { Logger, ConsoleBackend, LiveLogger } from 'clui-logger';
import { StatusCommand } from './Commands/StatusCommand';
import { QuitCommand } from './Commands/QuitCommand';
import { PlayCommand } from './Commands/PlayCommand';
import { Stopwatch } from 'data-stopwatch';
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

    protected preHooks : Map<string, CommandPreHook[]> = new Map();

    protected postHooks : Map<string, CommandPostHook[]> = new Map();

    protected globalPreHooks : CommandPreHook[] = [];

    protected globalPostHooks : CommandPostHook[] = [];
    
    constructor ( config ?: Config, logger ?: Logger ) {
        this.config = config || UnicastMpv.baseConfig();

        this.logger = logger || new Logger( new ConsoleBackend() );

        this.player = new Player( this.config.slice( 'player' ) );

        this.player.observeProperty( 'sub-scale' );
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

        this.registerGlobalPreHook( ( args, command, ctx : { stopwatch: Stopwatch, live : LiveLogger } ) => {
            ctx.stopwatch = new Stopwatch();
            ctx.live = rpcLogger.service( command ).live();

            ctx.live.debug( chalk.grey( `${ args.join( ' ' ) } running...` ) );

            ctx.stopwatch.resume();
        } );

        new NativeCommands( this );
        new StatusCommand( this );
        new QuitCommand( this );
        new PlayCommand( this );
        new StatusCommand( this );

        new Events( this );

        this.registerGlobalPostHook( ( args, command, error, result, ctx : { stopwatch: Stopwatch, live : LiveLogger } ) => {
            ctx.live.update( () => {
                ctx.live.debug( `${ args.join( ' ' ) } ${ ctx.stopwatch.readHumanized() } ${ error ? chalk.red( 'FAILED' ) : '' }` );

                if ( error && error.message ) {
                    ctx.live.error( error.message + ( error.stack ? ( '\n' + error.stack ) : '' ), error );
                } else if ( error && error.errcode && error.errmessage ) {
                    ctx.live.error( `CODE ${ error.errcode } ${ error.method }: ${ error.errmessage }`, error );
                }
            } );

            ctx.live.close();
        } );

        await new Promise( ( resolve, reject ) => {
            this.connection.on( 'listening', resolve )
    
            this.connection.on( 'error', reject );
        } );

        this.logger.info( 'Server listening on port ' + this.config.get( 'server.port' ) );
    }
}
