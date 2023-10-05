{
  description = "Fully free website for asking questions.";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
    devshell.url = "github:numtide/devshell";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs = inputs@{ self, nixpkgs, devshell, flake-parts }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      imports = [
        devshell.flakeModule
        flake-parts.flakeModules.easyOverlay
      ];

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      perSystem = { config, pkgs, final, ... }: {
        overlayAttrs = {
          inherit (config.packages)
            libreQuestions;
        };

        packages = {
          libreQuestions = pkgs.buildGo121Module rec {
            pname = "libre-questions";
            version = "0.0.0";

            src = ./.;
            subPackages = [ "cmd/libre-questions" ];

            vendorHash = "sha256-acLxvdGZv1eoxPJnGU9D2KqS9gSxPe4CUmf0BOVMGJY=";

            doCheck = false;

            CGO_ENABLED = 0;

            ldflags = [
              "-s"
              "-w"
              "-extldflags -static"
            ];
          };
        };

        devshells.default = {
          packages = with pkgs; [
            go_1_21
            golangci-lint
            modd
            sqlite
          ];
        };
      };

      flake = rec {
        nixosModules.default = nixosModules.libreQuestions;
        nixosModules.libreQuestions = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.libreQuestions;
          listenAddress = "${cfg.address}:${toString cfg.port}";
        in
        {
          options.services.libreQuestions = {
            enable = mkEnableOption (self.flake.description);

            hostName = mkOption {
              type = types.str;
              default = "localhost";
              description = "Hostname to serve the webserver.";
            };

            nginx = mkOption {
              type = types.attrs;
              default = { };
              example = literalExpression ''
                {
                  forceSSL = true;
                  enableACME = true;
                }
              '';
              description = "Extra configuration for the vhost. Useful for adding SSL settings.";
            };

            stateDir = mkOption {
              type = types.path;
              default = /var/lib/libre_questions;
              description = "Directory where the database will exist.";
            };

            databaseName = mkOption {
              type = types.str;
              default = "questions.db";
              description = "Name of the file within the `stateDir` for storing the data.";
            };

            user = mkOption {
              type = types.str;
              default = "librequestions";
              description = "User account under which the webserver runs.";
            };

            group = mkOption {
              type = types.str;
              default = "librequestions";
              description = "Group account under which the webserver runs.";
            };

            dynamicUser = mkOption {
              type = types.bool;
              default = true;
              description = ''
                Runs Libre Questions as a SystemD DynamicUser.
                It means SystenD will allocate the user at runtime, and enables
                some other security features.
                If you are not sure what this means, it's safe to leave it default.
              '';
            };

            address = mkOption {
              type = types.str;
              default = "127.0.0.1";
              description = "Listen address for the webserver.";
            };

            port = mkOption {
              type = types.port;
              default = 8080;
              description = "Listen port for the webserver.";
            };
          };

          config = mkIf cfg.enable {
            systemd.services = {
              libreQuestions = {
                description = "Libre Questions, the free questions webserver.";
                wantedBy = [ "multi-user.target" ];
                after = [ "network.target" ];

                serviceConfig = {
                  ExecStart =
                  let
                    args = [
                      "--db=${cfg.databaseName}"
                      "--listen=${listenAddress}"
                    ];
                  in
                  "${pkgs.libreQuestions}/bin/libre-questions ${concatStringsSep " " args}";

                  WorkingDirectory = cfg.stateDir;
                  StateDirectory = optional (cfg.stateDir == /var/lib/libre_questions) "libre_questions";

                  DynamicUser = cfg.dynamicUser;
                  Group = cfg.group;
                  User = cfg.user;

                  Restart = "on-failure";
                };
              };
            };

            services.nginx = {
              enable = true;
              upstreams.libreQuestions.servers."${listenAddress}" = { };
              virtualHosts."${cfg.hostName}" = mkMerge [
                cfg.nginx
                {
                  locations = {
                    "/" = {
                      index = "index.html";
                      proxyPass = "http://libreQuestions/";
                    };
                  };
                }
              ];
            };
          };
        };
      };
  };
}
