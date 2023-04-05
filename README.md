# routercommander

## Overview

**routercommander** is the tool developed to automate the process of reproducing issues and the collection a large number of commands from a router or a series of routers. As in example of a taking router's health check, the health check might require to run more than 50 commands, doing it one by one is pure insanity, as it will consume significant time; copy and pasting all commands and pray that there is no any syntax error or pasted commands will be correctly accepted by a router is no better option. **routercommander** will execute each command, collect the output and stored it in the file with a router name and the time stamp as a file name. In addition, **routercommander** allows to extend a bit the collection process by introducing several controlling parameters. For example some commands might need to be executed several time and with a specific time interval between them. Some commands might have a required *location* keyword some not. All these particularities can be controlled from the commands YAML file. Please see below a sample of such file for **show cef drop** command.

```yaml
main_command_group:
  - command: show cef drops
    times: 2
    interval: 10
    location:
      - "0/0/CPU0"
      - "0/1/CPU0"
      - "0/2/CPU0"
    patterns:
      - pattern_string: '((:?\w+\s)+)(drops\s+)(packets\s+:)\s+[1-9]\d*\n'
    debug: false
```

As it is clearly seen, this file defines **show cef drops** command.  It also defines a number of times  to execute it **2** as well as a time interval between in seconds **10**. It also instructs **routercommander** to run it only against locations 0/0/CPU0, 0/1/CPU0 and 0/2/CPU0.

the **pattern** keyword defines a pattern to detect an *alarming condition*, it is a part of a health check automation functionality which is still under the development.

## Command customization parameters

```yaml
main_command_group:
   - command:     < ----- defines a command to execute
     times:          < ----- defines number of times to execute this command,
                           make sense only in collect mode
     interval:       < ----- defines the interval in seconds between execution
                           of the command, make sense only in collect mode
     wait_before: number of seconds
     wait_after:  number of seconds 
     location:       < ----- defines a list of locations to execute the command
       - "0/0/CPU0"
     debug:          < ----- boolean true/false, used for debugging of
                             the execution of the command
     process_result: < ----- boolean true/false, by default in "collect" mode
                             results of commands are not processed, used to
                             override global value
     patterns:
        - pattern_string: < ----- defines a string representation of
                                  a regular expression to match
          capture:          < ----- if defined, used to capture a specific value
                                    and then compare between repro mode iterations
            field_number: [2,4] < ----- in the string matched by pattern_string,
                                    list of fields to capture
            separator: ":"  < ----- defines character ":" as a separator to use
                                    with the matched string to get field number 2 and 4 for example.
```

If only **pattern_string** tag present, without **capture**, then it will be treated just as a matching condition in the health check validation of **collect** mode, when both present, then they will be used to detect a value change between iterations of **repro** mode.

## 2 modes of routercommander operations "collect" and "repro"

**routercommander** can operate in two modes, ***collect*** and ***repro***. If **repro** section is present in the yaml file, **routercommander**  will switch to **repro** mode regardless if **collect** section also present.

### collect

In **collect** mode **routercommander** just collect the information based on the list of commands. All commands customization parameters listed above are available in **collect** mode. Some of them though, do not make sense as **Capture** section, which requires the presence of **repro** section. In **collect** section, health check (matching patterns of a command) can be globally enabled, by default it is disabled.

```yaml
collect:
   health_check:  < ----- boolean true/false
```

### repro

**repro** mode sets parameters of execution of a group of commands defined by **commands** section. It also defines a list of “post-mortem” commands to collect if the issue is triggered. Please see the example below:

```yaml
repro:
  times: 8640
  interval: 10
    command_processing_rules:
    #
    # command tag must match to one of the command tag from the main_command_group, under
    # this tag the special instructions for its processing are listed.
    - command: "run netstat -s -udp"
      patterns:
        #
        # the value of the pattern_string must match to one of the patterns defined for the command in the main_command_group.
        # If the pattern_string below had the capture tag in the main_command_group, then the captured
        # values would be available for the command mutation.
        - pattern_string: 'SndbufErrors:\s*[0-9+]'
          captured_values:
            - field_number: 2
              operation: "compare_with_previous_neq"
          pattern_commands:
            - command: "run netstat -aup | grep tcp"
  postmortem_command_group:
    - command: 'run for i in {1..20}; do date +"%T. %3N"; netstat -s -udp | grep SndbufErrors; netstat -aup | grep tcp; sleep 0.2; done'
main_command_group:
  - command: "run netstat -s -udp"
    collect_result: true
    patterns:
      - pattern_string: 'SndbufErrors:\s*[0-9+]'
        capture:
          field_number: [2]
          separator: ":"
    debug: false
```

In this example, commands defined by **commands:** tag, will be executed 8640 times with the interval of 10 seconds.  The repro is considered as triggered when the value of field 2 is changed between repro iterations. In this case commands defined by **postmortem_commands** tag will be executed.

Please see this [link](/testdata/commands_v2.md) for more detailed description of YAML file structure and parameters.

## To run

### as a linux binary

**routercommander** leverages the most ubiquitous access method used by network operators, ssh access. The mandatory parameters to run **routercommander** are:

- **--username** defines a user name to use to ssh to a router
- **--password** defines a password to use for ssh authentication
- **--command-file** defines the location of the commands YAML file

**routercommander** can execute the list of commands against a single router, for this case **--router-name** parameter should be used, or in a concurrent manner against a group of routers, the names of routers are stored in a normal text file:

```text
router1
router2
router3
router4
```

and **--routers-file** parameter defines its location.

```bash
routercommander --username=root --password=1234567 --router-name=router1 --command-file=./show_fib.yaml
```

the result of the routercommander execution will be a log file, named with router's name as a prefix and the timestamp of execution as suffix. The log file will container the output generated by the show command.

### as a docker container

Running **routercommander** as a container adds a small twist. Since we are passing 1 external file, the list of commands and expecting the container to create a log file on the external file system, we need to mount or map to the container  these two locations. It will become more clear after reviewing the example. All other parameters are exactly the same.

```bash
docker run --net=host -v /home/some-user/logs:/logs -v /home/nso:/testdata docker.io/sbezverk/routercommander:latest --router-name=router --username=user --password='pass' --v=5 --commands-file=./testdata/show_cef.yaml 
```

First volume we mount or map into the container is for the resulting log file, **-v /home/some-user/logs:/logs** this directive maps physical location **/home/some-user/logs** to the container's internal folder **/logs**,
second volume we map **-v /home/some-user/testdata:/testdata** to give the container access to the commands yaml file.

The resulting log file will be stored in **/home/some-user/logs** folder.
