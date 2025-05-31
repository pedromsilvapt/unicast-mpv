#!/usr/bin/env node

import { UnicastMpv, Config } from './index';
import path from 'path';
import os from 'os';

const configs = [
    UnicastMpv.baseConfig()
];

if ( process.argv.length > 2 ) {
    configs.push( Config.load( process.argv[ 2 ] ) );
} else {
    configs.push( Config.load( path.join( os.homedir(), 'unicast-mpv.yaml' ) ) );
}

const server = new UnicastMpv( Config.merge( configs ) );

process.on('unhandledRejection', error => {
    // Will print "unhandledRejection err is not defined"
    console.log('unhandledRejection', error);
});

server.listen()
    .catch( error => server.logger.fatal( error.message + '\n' + error.stack, error ) );